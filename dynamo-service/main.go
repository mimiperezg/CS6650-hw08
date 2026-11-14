package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/gin-gonic/gin"
)

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

func main() {
	table := os.Getenv("DYNAMODB_TABLE")
	if table == "" {
		log.Fatal("DYNAMODB_TABLE is not set")
	}
	consistentRead, err := strconv.ParseBool(os.Getenv("CONSISTENT_READ"))
	if err != nil {
		consistentRead = false
	}

	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	carts := &cartStore{
		db:             dynamodb.NewFromConfig(cfg),
		table:          table,
		consistentRead: consistentRead,
	}
	log.Printf("dynamo-service starting: table=%s consistent_read=%t", table, consistentRead)

	router := gin.New()
	router.Use(gin.CustomRecovery(func(c *gin.Context, _ any) {
		c.AbortWithStatusJSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "INTERNAL_ERROR",
			Message: "Internal server error",
		})
	}))
	router.POST("/v1/shopping-carts", carts.createCart)
	router.GET("/v1/shopping-carts/:id", carts.getCart)
	router.POST("/v1/shopping-carts/:id/items", carts.addItem)

	if err := router.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}

func invalidInput(c *gin.Context, details string) {
	c.JSON(http.StatusBadRequest, ErrorResponse{
		Error:   "INVALID_INPUT",
		Message: "The provided input data is invalid",
		Details: details,
	})
}

func cartNotFound(c *gin.Context, details string) {
	c.JSON(http.StatusNotFound, ErrorResponse{
		Error:   "CART_NOT_FOUND",
		Message: "Shopping cart not found",
		Details: details,
	})
}

func serverError(c *gin.Context, details string) {
	c.JSON(http.StatusInternalServerError, ErrorResponse{
		Error:   "INTERNAL_ERROR",
		Message: "Internal server error",
		Details: details,
	})
}
