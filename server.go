package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

var (
	DWOLLA_APP_KEY        = ""
	DWOLLA_APP_SECRET     = ""
	DWOLLA_ENV            = ""
	DWOLLA_BASE_URL       = ""
	PLAID_API_URL         = ""
	APP_PORT              = ""
	DWOLLA_WEBHOOK_SECRET = ""
	WEBHOOK_BASE_URL      = ""
	dwollaToken           = ""

	// Webhook events storage
	webhookEvents []map[string]interface{}
	webhookMutex  sync.RWMutex
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
	DWOLLA_WEBHOOK_SECRET = os.Getenv("DWOLLA_WEBHOOK_SECRET")
	WEBHOOK_BASE_URL = os.Getenv("WEBHOOK_BASE_URL")

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

	// Webhook endpoints
	r.POST("/api/dwolla/webhook-subscription", createWebhookSubscription)
	r.GET("/api/dwolla/webhook-subscriptions", listWebhookSubscriptions)
	r.DELETE("/api/dwolla/webhook-subscription/:id", deleteWebhookSubscription)
	r.POST("/api/dwolla/webhook", handleWebhook)
	r.GET("/api/dwolla/webhook-events", getWebhookEvents)

	// Sandbox simulation endpoints
	r.POST("/api/dwolla/simulate-transfer", simulateTransfer)

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

// createWebhookSubscription creates or updates a webhook subscription
// POST /api/dwolla/webhook-subscription
func createWebhookSubscription(c *gin.Context) {
	var reqBody struct {
		URL    string `json:"url"`
		Secret string `json:"secret"`
	}

	if err := c.BindJSON(&reqBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Use environment variables if not provided in request
	webhookURL := reqBody.URL
	if webhookURL == "" {
		if WEBHOOK_BASE_URL == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "webhook URL not provided and WEBHOOK_BASE_URL not set"})
			return
		}
		webhookURL = WEBHOOK_BASE_URL + "/api/dwolla/webhook"
	}

	webhookSecret := reqBody.Secret
	if webhookSecret == "" {
		webhookSecret = DWOLLA_WEBHOOK_SECRET
	}

	if webhookSecret == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "webhook secret not provided and DWOLLA_WEBHOOK_SECRET not set"})
		return
	}

	// Create webhook subscription payload
	payload := map[string]interface{}{
		"url":    webhookURL,
		"secret": webhookSecret,
	}

	url := DWOLLA_BASE_URL + "/webhook-subscriptions"
	result, status, err := makeDwollaRequest("POST", url, payload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if status != http.StatusCreated {
		c.JSON(status, gin.H{"error": "Failed to create webhook subscription", "details": result})
		return
	}

	subscriptionURL := result["location"].(string)
	fmt.Printf("✓ Created webhook subscription: %s\n", subscriptionURL)
	fmt.Printf("  Webhook URL: %s\n", webhookURL)

	c.JSON(http.StatusOK, gin.H{
		"subscription_url": subscriptionURL,
		"webhook_url":      webhookURL,
		"status":           "created",
	})
}

// listWebhookSubscriptions lists all webhook subscriptions
// GET /api/dwolla/webhook-subscriptions
func listWebhookSubscriptions(c *gin.Context) {
	url := DWOLLA_BASE_URL + "/webhook-subscriptions"
	result, status, err := makeDwollaRequest("GET", url, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if status != http.StatusOK {
		c.JSON(status, gin.H{"error": "Failed to list webhook subscriptions", "details": result})
		return
	}

	c.JSON(http.StatusOK, result)
}

// deleteWebhookSubscription deletes a webhook subscription
// DELETE /api/dwolla/webhook-subscription/:id
func deleteWebhookSubscription(c *gin.Context) {
	subscriptionID := c.Param("id")

	url := DWOLLA_BASE_URL + "/webhook-subscriptions/" + subscriptionID
	result, status, err := makeDwollaRequest("DELETE", url, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if status != http.StatusOK && status != http.StatusNoContent {
		c.JSON(status, gin.H{"error": "Failed to delete webhook subscription", "details": result})
		return
	}

	fmt.Printf("✓ Deleted webhook subscription: %s\n", subscriptionID)

	c.JSON(http.StatusOK, gin.H{
		"status":          "deleted",
		"subscription_id": subscriptionID,
	})
}

// verifyWebhookSignature verifies the Dwolla webhook signature
func verifyWebhookSignature(signature, payload, secret string) bool {
	if secret == "" {
		fmt.Println("⚠ Warning: DWOLLA_WEBHOOK_SECRET not set, skipping signature verification")
		return true
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	expectedSignature := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

// handleWebhook receives and processes Dwolla webhook notifications
// POST /api/dwolla/webhook
func handleWebhook(c *gin.Context) {
	// Read raw body for signature verification
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}

	// Get signature from header
	signature := c.GetHeader("X-Request-Signature-SHA-256")

	// Verify signature
	if !verifyWebhookSignature(signature, string(bodyBytes), DWOLLA_WEBHOOK_SECRET) {
		fmt.Printf("❌ Webhook signature verification failed\n")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid signature"})
		return
	}

	// Parse webhook payload
	var webhook map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &webhook); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON payload"})
		return
	}

	// Extract webhook details
	eventID, _ := webhook["id"].(string)
	topic, _ := webhook["topic"].(string)
	timestamp, _ := webhook["timestamp"].(string)

	// Store webhook event
	webhookMutex.Lock()
	webhookEvents = append(webhookEvents, webhook)
	// Keep only last 50 events to prevent memory issues
	if len(webhookEvents) > 50 {
		webhookEvents = webhookEvents[len(webhookEvents)-50:]
	}
	webhookMutex.Unlock()

	// Log webhook event
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Printf("🔔 WEBHOOK RECEIVED at %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Event ID:  %s\n", eventID)
	fmt.Printf("Topic:     %s\n", topic)
	fmt.Printf("Timestamp: %s\n", timestamp)

	// Extract resource links
	if links, ok := webhook["_links"].(map[string]interface{}); ok {
		if resource, ok := links["resource"].(map[string]interface{}); ok {
			if resourceHref, ok := resource["href"].(string); ok {
				fmt.Printf("Resource:  %s\n", resourceHref)
			}
		}
	}

	// Handle specific event types
	switch topic {
	case "transfer_completed":
		fmt.Println("✅ Transfer completed successfully!")
	case "transfer_failed":
		fmt.Println("❌ Transfer failed!")
	case "transfer_cancelled":
		fmt.Println("⚠ Transfer cancelled!")
	case "customer_created":
		fmt.Println("👤 Customer created")
	case "customer_funding_source_added":
		fmt.Println("🏦 Funding source added")
	case "customer_funding_source_verified":
		fmt.Println("✓ Funding source verified")
	default:
		fmt.Printf("ℹ Event: %s\n", topic)
	}

	// Print full webhook payload for debugging
	fmt.Println("\nFull webhook payload:")
	prettyJSON, _ := json.MarshalIndent(webhook, "", "  ")
	fmt.Println(string(prettyJSON))
	fmt.Println(strings.Repeat("=", 60))

	// Respond with 200 OK to acknowledge receipt
	c.JSON(http.StatusOK, gin.H{
		"status":   "received",
		"event_id": eventID,
		"topic":    topic,
	})
}

// simulateTransfer simulates transfer processing in sandbox environment
// POST /api/dwolla/simulate-transfer
func simulateTransfer(c *gin.Context) {
	var reqBody struct {
		TransferURL string `json:"transfer_url" binding:"required"`
		Action      string `json:"action"` // "process" (complete) or "fail"
	}

	if err := c.BindJSON(&reqBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Set default action to process (complete the transfer)
	action := reqBody.Action
	if action == "" {
		action = "process"
	}

	// Validate action
	if action != "process" && action != "fail" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "action must be 'process' or 'fail'"})
		return
	}

	// Create simulation payload
	payload := map[string]interface{}{
		"_links": map[string]interface{}{
			"transfer": map[string]string{
				"href": reqBody.TransferURL,
			},
		},
	}

	// For process action, simulate processing
	if action == "process" {
		// Process the transfer to completion
	} else if action == "fail" {
		// Add failure code for failed transfers
		payload["failureCode"] = "R01" // Insufficient Funds
	}

	url := DWOLLA_BASE_URL + "/sandbox-simulations"
	result, status, err := makeDwollaRequest("POST", url, payload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if status != http.StatusCreated && status != http.StatusOK {
		c.JSON(status, gin.H{"error": "Failed to simulate transfer", "details": result})
		return
	}

	actionMsg := "completed"
	if action == "fail" {
		actionMsg = "failed"
	}

	fmt.Printf("✓ Simulated transfer %s: %s\n", actionMsg, reqBody.TransferURL)
	fmt.Printf("  💡 Check webhook logs for transfer_%s event\n", actionMsg)

	c.JSON(http.StatusOK, gin.H{
		"status":       "simulated",
		"action":       action,
		"transfer_url": reqBody.TransferURL,
		"message":      fmt.Sprintf("Transfer simulation initiated. Webhook should trigger transfer_%s event.", actionMsg),
	})
}

// getWebhookEvents returns the list of received webhook events
// GET /api/dwolla/webhook-events
func getWebhookEvents(c *gin.Context) {
	webhookMutex.RLock()
	defer webhookMutex.RUnlock()

	// Return a copy of the events
	events := make([]map[string]interface{}, len(webhookEvents))
	copy(events, webhookEvents)

	c.JSON(http.StatusOK, events)
}
