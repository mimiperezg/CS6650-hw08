package main

import (
	"database/sql"
	"net/http"
	"github.com/gin-gonic/gin"
)

// gives cart handlers access to the database
// it is the same way that productStore gives handlers access to the product map
type cartStore struct {
	db *sql.DB
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



