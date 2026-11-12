package main

import (
	"database/sql"
	"net/http"
	"github.com/gin-gonic/gin"
	"strconv"
	"fmt"
)

// gives cart handlers access to the database
// it is the same way that productStore gives handlers access to the product map
type cartStore struct {
	db *sql.DB
}

// one item in the GET response
type cartItem struct {
	ProductID int32 `json:"product_id"`
	Quantity  int32 `json:"quantity"`
}

// full GET response
type cartResponse struct {
	ShoppingCartID int32      `json:"shopping_cart_id"`
	CustomerID     int32      `json:"customer_id"`
	Items          []cartItem `json:"items"`
}

// handles the GET /v1/shopping-carts/:id
func (s *cartStore) getCart(c *gin.Context) {
	// parse id from the path
	id, err := strconv.ParseInt(c.Param("id"), 10, 32)
	// if error or id is less than 1 its not valid
	if err != nil || id < 1 {
		invalidInput(c, "id must be a positive 32-bit integer")
		return
	}
	// run query with LEFT JOIN
	rows, err := s.db.Query(`
		SELECT c.id, c.customer_id, i.product_id, i.quantity
		FROM carts c
		LEFT JOIN cart_items i ON i.cart_id = c.id
		WHERE c.id = ?`, id)
	if err != nil {
		serverError(c, err.Error())
		return
	}
	defer rows.Close()

	// response to be filled in
	resp := cartResponse{Items: []cartItem{}}
	// tracks whether a row came back
	found := false

	// loop through rows
	for rows.Next() {
		var productID, quantity sql.NullInt32
		if err := rows.Scan(&resp.ShoppingCartID, &resp.CustomerID, &productID, &quantity); err != nil {
			serverError(c, err.Error())
			return
		}
		found = true
		if productID.Valid {
			resp.Items = append(resp.Items, cartItem{
				ProductID: productID.Int32,
				Quantity:  quantity.Int32,
			})
		}
	}
			// check for errors
	if err := rows.Err(); err != nil {
		serverError(c, err.Error())
		return
	}

	// return 404
	if !found {
		cartNotFound(c, fmt.Sprintf("no shopping cart with ID %d", id))
		return
	}
	// return 200
	c.JSON(http.StatusOK, resp)

}

// this matches the request body in the api.yaml
type createCartRequest struct {
	CustomerID int32 `json:"customer_id" binding:"required,min=1"`
}

// handles the POST /v1/shopping-carts
func (s *cartStore) createCart(c *gin.Context) {
	// declare createCartRequest and bind json body to it
	var req createCartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		invalidInput(c, err.Error())
		return
	}
	// insert cart into the database
	result, err := s.db.Exec("INSERT INTO carts (customer_id) VALUES (?)", req.CustomerID)
	if err != nil {
		serverError(c, err.Error())
		return
	}

	// get new id with result.LastInsertId()
	var newID int32
	id, err := result.LastInsertId()
	if err != nil {
		serverError(c, err.Error())
		return
	}
	newID = int32(id)

	// respond with 201
	c.JSON(http.StatusCreated, gin.H{"shopping_cart_id": newID})
}

// sends 500 in the spec's error shape
func serverError(c *gin.Context, details string) {
	c.JSON(http.StatusInternalServerError, ErrorResponse{
		Error: "INTERNAL_ERROR",
		Message: "Internal Server Error",
		Details: details,
	})
}

func cartNotFound(c *gin.Context, details string) {
	c.JSON(http.StatusNotFound, ErrorResponse{
		Error: "CART_NOT_FOUND",
		Message: "Shopping cart Not Found",
		Details: details,
	})
}



