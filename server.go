package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

var (
	DWOLLA_APP_KEY    = ""
	DWOLLA_APP_SECRET = ""
	DWOLLA_ENV        = ""
	DWOLLA_BASE_URL   = ""
	PLAID_API_URL     = ""
	APP_PORT          = ""
	dwollaToken       = ""
)

func init() {
	// Load env vars from .env file
	err := godotenv.Load()
	if err != nil {
		fmt.Println("Error when loading environment variables from .env file:", err)
	}

	// Set constants from env
	DWOLLA_APP_KEY = os.Getenv("DWOLLA_APP_KEY")
	DWOLLA_APP_SECRET = os.Getenv("DWOLLA_APP_SECRET")
	DWOLLA_ENV = os.Getenv("DWOLLA_ENV")
	DWOLLA_BASE_URL = os.Getenv("DWOLLA_BASE_URL")
	PLAID_API_URL = os.Getenv("PLAID_API_URL")
	APP_PORT = os.Getenv("APP_PORT")

	// Set defaults
	if DWOLLA_ENV == "" {
		DWOLLA_ENV = "sandbox"
	}
	if DWOLLA_BASE_URL == "" {
		DWOLLA_BASE_URL = "https://api-sandbox.dwolla.com"
	}
	if PLAID_API_URL == "" {
		PLAID_API_URL = "http://localhost:8000"
	}
	if APP_PORT == "" {
		APP_PORT = "8001"
	}

	// Validate required env vars
	if DWOLLA_APP_KEY == "" || DWOLLA_APP_SECRET == "" {
		log.Fatal("Error: DWOLLA_APP_KEY or DWOLLA_APP_SECRET is not set. Did you copy .env.example to .env and fill it out?")
	}

	fmt.Printf("Dwolla environment: %s\n", DWOLLA_ENV)
	fmt.Printf("Dwolla base URL: %s\n", DWOLLA_BASE_URL)
	fmt.Printf("Plaid API URL: %s\n", PLAID_API_URL)

	// Get Dwolla access token
	token, err := getDwollaToken()
	if err != nil {
		log.Fatal("Failed to get Dwolla access token:", err)
	}
	dwollaToken = token
	fmt.Printf("Dwolla token obtained successfully\n")
}

func main() {
	r := gin.Default()

	// Health check endpoint
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":     "ok",
			"dwolla_env": DWOLLA_ENV,
			"plaid_url":  PLAID_API_URL,
			"dwolla_url": DWOLLA_BASE_URL,
		})
	})

	// Dwolla endpoints
	r.GET("/api/dwolla/accounts", getAccounts)
	r.POST("/api/dwolla/customer", createCustomer)
	r.POST("/api/dwolla/funding-source", createFundingSource)
	r.POST("/api/dwolla/transfer", createTransfer)
	r.GET("/api/dwolla/transfer/:id", getTransfer)

	fmt.Printf("Dwolla Transfer Demo server starting on port %s...\n", APP_PORT)
	err := r.Run(":" + APP_PORT)
	if err != nil {
		log.Fatal("Unable to start server:", err)
	}
}

// getDwollaToken gets an OAuth access token from Dwolla
func getDwollaToken() (string, error) {
	authURL := DWOLLA_BASE_URL + "/token"

	// Create request body
	data := "grant_type=client_credentials"

	req, err := http.NewRequest("POST", authURL, strings.NewReader(data))
	if err != nil {
		return "", err
	}

	// Set headers
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(DWOLLA_APP_KEY, DWOLLA_APP_SECRET)

	// Make request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to get token, status: %d, body: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.AccessToken, nil
}

// makeDwollaRequest makes an authenticated request to Dwolla API
func makeDwollaRequest(method, url string, body interface{}) (map[string]interface{}, int, error) {
	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, 0, err
	}

	req.Header.Set("Authorization", "Bearer "+dwollaToken)
	req.Header.Set("Content-Type", "application/vnd.dwolla.v1.hal+json")
	req.Header.Set("Accept", "application/vnd.dwolla.v1.hal+json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}

	var result map[string]interface{}
	if len(bodyBytes) > 0 {
		if err := json.Unmarshal(bodyBytes, &result); err != nil {
			return nil, resp.StatusCode, fmt.Errorf("failed to parse response: %s", string(bodyBytes))
		}
	}

	// For 201 Created responses, get the Location header
	if resp.StatusCode == http.StatusCreated {
		location := resp.Header.Get("Location")
		if location != "" && result == nil {
			result = map[string]interface{}{"location": location}
		} else if location != "" {
			result["location"] = location
		}
	}

	return result, resp.StatusCode, nil
}

// getProcessorToken calls Plaid API to get a processor token
func getProcessorToken() (string, error) {
	url := PLAID_API_URL + "/api/sandbox/processor_token"

	req, err := http.NewRequest("POST", url, bytes.NewBuffer([]byte("{}")))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to get processor token, status: %d, body: %s", resp.StatusCode, string(body))
	}

	var result struct {
		ProcessorToken string `json:"processor_token"`
		AccountID      string `json:"account_id"`
		ItemID         string `json:"item_id"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	fmt.Printf("Got processor_token from Plaid: %s\n", result.ProcessorToken)
	return result.ProcessorToken, nil
}

// getAccounts gets Dwolla root/master account information
// GET /api/dwolla/accounts
func getAccounts(c *gin.Context) {
	url := DWOLLA_BASE_URL
	result, status, err := makeDwollaRequest("GET", url, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if status != http.StatusOK {
		c.JSON(status, gin.H{"error": "Failed to get accounts", "details": result})
		return
	}

	// Extract account URL from _links
	links, ok := result["_links"].(map[string]interface{})
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid response format"})
		return
	}

	account, ok := links["account"].(map[string]interface{})
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Account link not found"})
		return
	}

	accountHref, ok := account["href"].(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Account href not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"account_url": accountHref,
		"_links":      links,
	})
}

// createCustomer creates a Dwolla customer
// POST /api/dwolla/customer
func createCustomer(c *gin.Context) {
	var reqBody struct {
		FirstName string `json:"firstName" binding:"required"`
		LastName  string `json:"lastName" binding:"required"`
		Email     string `json:"email" binding:"required"`
	}

	if err := c.BindJSON(&reqBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Create customer payload
	payload := map[string]interface{}{
		"firstName": reqBody.FirstName,
		"lastName":  reqBody.LastName,
		"email":     reqBody.Email,
	}

	url := DWOLLA_BASE_URL + "/customers"
	result, status, err := makeDwollaRequest("POST", url, payload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if status != http.StatusCreated {
		c.JSON(status, gin.H{"error": "Failed to create customer", "details": result})
		return
	}

	customerURL := result["location"].(string)
	fmt.Printf("Created customer: %s\n", customerURL)

	c.JSON(http.StatusOK, gin.H{
		"customer_url": customerURL,
		"status":       "created",
	})
}

// createFundingSource adds a bank account as a funding source
// POST /api/dwolla/funding-source
func createFundingSource(c *gin.Context) {
	var reqBody struct {
		CustomerURL string `json:"customer_url" binding:"required"`
		Name        string `json:"name"`
	}

	if err := c.BindJSON(&reqBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get processor token from Plaid
	processorToken, err := getProcessorToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to get processor token from Plaid",
			"details": err.Error(),
		})
		return
	}

	// Set default name if not provided
	name := reqBody.Name
	if name == "" {
		name = "Bank Account"
	}

	// Create funding source payload
	payload := map[string]interface{}{
		"plaidToken": processorToken,
		"name":       name,
	}

	url := reqBody.CustomerURL + "/funding-sources"
	result, status, err := makeDwollaRequest("POST", url, payload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if status != http.StatusCreated {
		c.JSON(status, gin.H{"error": "Failed to create funding source", "details": result})
		return
	}

	fundingSourceURL := result["location"].(string)
	fmt.Printf("Created funding source: %s\n", fundingSourceURL)

	c.JSON(http.StatusOK, gin.H{
		"funding_source_url": fundingSourceURL,
		"status":             "created",
	})
}

// createTransfer initiates a transfer
// POST /api/dwolla/transfer
func createTransfer(c *gin.Context) {
	var reqBody struct {
		Source      string  `json:"source" binding:"required"`
		Destination string  `json:"destination" binding:"required"`
		Amount      float64 `json:"amount" binding:"required"`
		Currency    string  `json:"currency"`
	}

	if err := c.BindJSON(&reqBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Set default currency
	currency := reqBody.Currency
	if currency == "" {
		currency = "USD"
	}

	// Create transfer payload
	payload := map[string]interface{}{
		"_links": map[string]interface{}{
			"source": map[string]string{
				"href": reqBody.Source,
			},
			"destination": map[string]string{
				"href": reqBody.Destination,
			},
		},
		"amount": map[string]interface{}{
			"currency": currency,
			"value":    fmt.Sprintf("%.2f", reqBody.Amount),
		},
	}

	url := DWOLLA_BASE_URL + "/transfers"
	result, status, err := makeDwollaRequest("POST", url, payload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if status != http.StatusCreated {
		c.JSON(status, gin.H{"error": "Failed to create transfer", "details": result})
		return
	}

	transferURL := result["location"].(string)
	fmt.Printf("Created transfer: %s\n", transferURL)

	c.JSON(http.StatusOK, gin.H{
		"transfer_url": transferURL,
		"status":       "created",
	})
}

// getTransfer retrieves transfer details
// GET /api/dwolla/transfer/:id
func getTransfer(c *gin.Context) {
	transferID := c.Param("id")

	url := DWOLLA_BASE_URL + "/transfers/" + transferID
	result, status, err := makeDwollaRequest("GET", url, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if status != http.StatusOK {
		c.JSON(status, gin.H{"error": "Failed to get transfer", "details": result})
		return
	}

	c.JSON(http.StatusOK, result)
}
