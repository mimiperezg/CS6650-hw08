package main

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/gin-gonic/gin"
)

const (
	cartTTL           = 7 * 24 * time.Hour
	maxCreateAttempts = 5
)

type cartStore struct {
	db             *dynamodb.Client
	table          string
	consistentRead bool
}

type cartItem struct {
	ProductID int32 `json:"product_id"`
	Quantity  int32 `json:"quantity"`
}

type cartResponse struct {
	ShoppingCartID int32      `json:"shopping_cart_id"`
	CustomerID     int32      `json:"customer_id"`
	Items          []cartItem `json:"items"`
}

type createCartRequest struct {
	CustomerID int32 `json:"customer_id" binding:"required,min=1"`
}

type addItemRequest struct {
	ProductID int32 `json:"product_id" binding:"required,min=1"`
	Quantity  int32 `json:"quantity" binding:"required,min=1"`
}

func (s *cartStore) createCart(c *gin.Context) {
	var req createCartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		invalidInput(c, err.Error())
		return
	}

	now := time.Now().UTC()
	for attempt := 0; attempt < maxCreateAttempts; attempt++ {
		cartID := rand.Int32N(math.MaxInt32) + 1
		_, err := s.db.PutItem(c.Request.Context(), &dynamodb.PutItemInput{
			TableName: aws.String(s.table),
			Item: map[string]types.AttributeValue{
				"cart_id":     number(int64(cartID)),
				"customer_id": number(int64(req.CustomerID)),
				"items":       &types.AttributeValueMemberM{Value: map[string]types.AttributeValue{}},
				"created_at":  &types.AttributeValueMemberS{Value: now.Format(time.RFC3339)},
				"expires_at":  number(now.Add(cartTTL).Unix()),
			},
			ConditionExpression: aws.String("attribute_not_exists(cart_id)"),
		})
		var taken *types.ConditionalCheckFailedException
		if errors.As(err, &taken) {
			continue
		}
		if err != nil {
			serverError(c, err.Error())
			return
		}
		c.JSON(http.StatusCreated, gin.H{"shopping_cart_id": cartID})
		return
	}
	serverError(c, "could not find an unused shopping cart id")
}

func (s *cartStore) addItem(c *gin.Context) {
	id, ok := parseCartID(c)
	if !ok {
		return
	}

	var req addItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		invalidInput(c, err.Error())
		return
	}

	_, err := s.db.UpdateItem(c.Request.Context(), &dynamodb.UpdateItemInput{
		TableName:           aws.String(s.table),
		Key:                 cartKey(id),
		UpdateExpression:    aws.String("SET #items.#p = if_not_exists(#items.#p, :zero) + :q"),
		ConditionExpression: aws.String("attribute_exists(cart_id)"),
		ExpressionAttributeNames: map[string]string{
			"#items": "items",
			"#p":     strconv.Itoa(int(req.ProductID)),
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":zero": number(0),
			":q":    number(int64(req.Quantity)),
		},
	})
	var missing *types.ConditionalCheckFailedException
	if errors.As(err, &missing) {
		cartNotFound(c, fmt.Sprintf("no shopping cart with ID %d", id))
		return
	}
	if err != nil {
		serverError(c, err.Error())
		return
	}

	c.Status(http.StatusNoContent)
}

func (s *cartStore) getCart(c *gin.Context) {
	id, ok := parseCartID(c)
	if !ok {
		return
	}

	out, err := s.db.GetItem(c.Request.Context(), &dynamodb.GetItemInput{
		TableName:      aws.String(s.table),
		Key:            cartKey(id),
		ConsistentRead: aws.Bool(s.consistentRead),
	})
	if err != nil {
		serverError(c, err.Error())
		return
	}
	if out.Item == nil {
		cartNotFound(c, fmt.Sprintf("no shopping cart with ID %d", id))
		return
	}

	customerID, err := readNumber(out.Item["customer_id"])
	if err != nil {
		serverError(c, "malformed customer_id: "+err.Error())
		return
	}

	resp := cartResponse{
		ShoppingCartID: int32(id),
		CustomerID:     int32(customerID),
		Items:          []cartItem{},
	}

	if itemsAttr, ok := out.Item["items"].(*types.AttributeValueMemberM); ok {
		for key, value := range itemsAttr.Value {
			productID, err := strconv.ParseInt(key, 10, 32)
			if err != nil {
				serverError(c, "malformed product id: "+key)
				return
			}
			quantity, err := readNumber(value)
			if err != nil {
				serverError(c, "malformed quantity for product "+key)
				return
			}
			resp.Items = append(resp.Items, cartItem{
				ProductID: int32(productID),
				Quantity:  int32(quantity),
			})
		}
	}
	sort.Slice(resp.Items, func(i, j int) bool {
		return resp.Items[i].ProductID < resp.Items[j].ProductID
	})

	c.JSON(http.StatusOK, resp)
}

func parseCartID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 32)
	if err != nil || id < 1 {
		invalidInput(c, "id must be a positive 32-bit integer")
		return 0, false
	}
	return id, true
}

func cartKey(id int64) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{"cart_id": number(id)}
}

func number(v int64) types.AttributeValue {
	return &types.AttributeValueMemberN{Value: strconv.FormatInt(v, 10)}
}

func readNumber(av types.AttributeValue) (int64, error) {
	n, ok := av.(*types.AttributeValueMemberN)
	if !ok {
		return 0, errors.New("not a number attribute")
	}
	return strconv.ParseInt(n.Value, 10, 64)
}
