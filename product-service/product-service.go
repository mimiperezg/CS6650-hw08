package main

import (
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// metricsPath is where Prometheus scrapes the service's metrics.
const metricsPath = "/metrics"

// Product matches the Product schema in api.yaml. The binding tags enforce
// the schema's required fields and min/max constraints, and the int32 types
// reject non-integers and values outside the int32 range.
type Product struct {
	ProductID    int32  `json:"product_id" binding:"required,min=1"`
	SKU          string `json:"sku" binding:"required,min=1,max=100"`
	Manufacturer string `json:"manufacturer" binding:"required,min=1,max=200"`
	CategoryID   int32  `json:"category_id" binding:"required,min=1"`
	// Weight is a pointer because 0 grams is valid, and "required" on a plain
	// int32 would reject 0. A nil pointer means the field was missing.
	Weight      *int32 `json:"weight" binding:"required,min=0"`
	SomeOtherID int32  `json:"some_other_id" binding:"required,min=1"`
}

// ErrorResponse matches the Error schema in api.yaml.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// productStore keeps products in memory, keyed by product ID. Gin serves
// each request on its own goroutine, so the map is guarded by a RWMutex.
type productStore struct {
	mu       sync.RWMutex
	products map[int32]Product
}

func newProductStore() *productStore {
	return &productStore{products: make(map[int32]Product)}
}

func (s *productStore) get(id int32) (Product, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.products[id]
	return p, ok
}

func (s *productStore) put(p Product) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.products[p.ProductID] = p
}

func main() {
	// open database connection
	db, err := openDB()
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// create tables if they don't exist
	if err := createTables(db); err != nil {
		log.Fatal(err)
	}

	router := setupRouter(newProductStore(), &cartStore{db: db})
	if err := router.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}

func setupRouter(store *productStore, carts *cartStore) *gin.Engine {
	router := gin.New()
	// One JSON log line per request, to stdout. On ECS, the awslogs driver
	// sends stdout to CloudWatch, where Logs Insights parses the JSON fields.
	router.Use(observeRequests(slog.New(slog.NewJSONHandler(os.Stdout, nil))))
	// Report panics with the spec's Error body instead of Gin's empty 500.
	router.Use(gin.CustomRecovery(func(c *gin.Context, _ any) {
		c.AbortWithStatusJSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "INTERNAL_ERROR",
			Message: "Internal server error",
		})
	}))

	router.GET("/v1/products/:productId", store.getProduct)
	router.POST("/v1/products/:productId/details", store.addProductDetails)
	router.POST("/v1/shopping-carts", carts.createCart)
	router.GET("/v1/shopping-carts/:id", carts.getCart)
	router.GET(metricsPath, gin.WrapH(promhttp.Handler()))
	return router
}

// getProduct handles GET /products/{productId}.
func (s *productStore) getProduct(c *gin.Context) {
	id, ok := parseProductID(c)
	if !ok {
		// api.yaml defines no 400 for this endpoint, so a malformed ID is
		// reported as 404: no product can exist under it.
		notFound(c, "productId must be a positive 32-bit integer")
		return
	}

	product, ok := s.get(id)
	if !ok {
		notFound(c, fmt.Sprintf("no product with ID %d", id))
		return
	}
	c.JSON(http.StatusOK, product)
}

// addProductDetails handles POST /products/{productId}/details. The spec
// describes it as "add or update", so a product that doesn't exist yet is
// created rather than rejected with 404.
func (s *productStore) addProductDetails(c *gin.Context) {
	id, ok := parseProductID(c)
	if !ok {
		invalidInput(c, "productId must be a positive 32-bit integer")
		return
	}

	var product Product
	if err := c.ShouldBindJSON(&product); err != nil {
		invalidInput(c, err.Error())
		return
	}
	if product.ProductID != id {
		invalidInput(c, fmt.Sprintf("product_id in body (%d) does not match productId in path (%d)", product.ProductID, id))
		return
	}

	s.put(product)
	c.Status(http.StatusNoContent)
}

// parseProductID reads the productId path parameter, which api.yaml defines
// as an int32 with a minimum of 1.
func parseProductID(c *gin.Context) (int32, bool) {
	id, err := strconv.ParseInt(c.Param("productId"), 10, 32)
	if err != nil || id < 1 {
		return 0, false
	}
	return int32(id), true
}

func invalidInput(c *gin.Context, details string) {
	c.JSON(http.StatusBadRequest, ErrorResponse{
		Error:   "INVALID_INPUT",
		Message: "The provided input data is invalid",
		Details: details,
	})
}

func notFound(c *gin.Context, details string) {
	c.JSON(http.StatusNotFound, ErrorResponse{
		Error:   "PRODUCT_NOT_FOUND",
		Message: "Product not found",
		Details: details,
	})
}
