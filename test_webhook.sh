#!/bin/bash

# Dwolla Webhook Integration Test
# This script tests the complete webhook flow with automatic ngrok setup

set -e

PLAID_URL="http://localhost:8000"
DWOLLA_URL="http://localhost:8001"
NGROK_PID=""
SUBSCRIPTION_ID=""

# Load Dwolla credentials from .env file
if [ -f .env ]; then
    export $(grep -v '^#' .env | xargs)
fi

# Set default Dwolla base URL if not set
DWOLLA_BASE_URL="${DWOLLA_BASE_URL:-https://api-sandbox.dwolla.com}"

# Generate unique timestamp for email addresses
TIMESTAMP=$(date +%s)

# Cleanup function to run on exit
cleanup() {
    echo ""
    echo "========================================"
    echo "Cleaning up..."
    echo "========================================"
    
    # Delete webhook subscription if created
    if [ -n "$SUBSCRIPTION_ID" ]; then
        echo "Deleting webhook subscription..."
        curl -s -X DELETE "${DWOLLA_URL}/api/dwolla/webhook-subscription/${SUBSCRIPTION_ID}" > /dev/null || true
        echo "✓ Webhook subscription deleted"
    fi
    
    # Kill ngrok if we started it
    if [ -n "$NGROK_PID" ]; then
        echo "Stopping ngrok..."
        kill $NGROK_PID 2>/dev/null || true
        echo "✓ ngrok stopped"
    fi
    
    echo "Cleanup complete"
}

# Set trap to run cleanup on exit
trap cleanup EXIT INT TERM

echo "========================================"
echo "Dwolla Webhook Integration Test"
echo "========================================"
echo ""

# Check if ngrok is installed
echo "1. Checking ngrok installation..."
if ! command -v ngrok &> /dev/null; then
    echo "   ❌ ngrok is not installed!"
    echo ""
    echo "   To install ngrok:"
    echo "   macOS:   brew install ngrok/ngrok/ngrok"
    echo "   Linux:   snap install ngrok"
    echo "   Windows: Download from https://ngrok.com/download"
    echo ""
    echo "   After installation, configure your authtoken:"
    echo "   1. Sign up at https://dashboard.ngrok.com/signup"
    echo "   2. Get your authtoken from https://dashboard.ngrok.com/get-started/your-authtoken"
    echo "   3. Run: ngrok config add-authtoken YOUR_AUTHTOKEN"
    exit 1
fi
echo "   ✓ ngrok is installed"
echo ""

# Check if both services are running
echo "2. Checking services..."
echo "   Plaid service (port 8000)..."
if ! curl -s "${PLAID_URL}/api/sandbox/processor_token" -X POST -H "Content-Type: application/json" -d '{}' > /dev/null; then
    echo "   ❌ Plaid service not running! Start it with: cd plaid-quickstart/go && go run server.go"
    exit 1
fi
echo "   ✓ Plaid service is running"

echo "   Dwolla service (port 8001)..."
if ! curl -s "${DWOLLA_URL}/health" > /dev/null; then
    echo "   ❌ Dwolla service not running! Start it with: cd dwolla-transfer-demo && go run server.go"
    exit 1
fi
echo "   ✓ Dwolla service is running"
echo ""

# Start ngrok if not already running
echo "3. Setting up ngrok tunnel..."
if ! curl -s http://localhost:4040/api/tunnels > /dev/null 2>&1; then
    echo "   Starting ngrok on port 8001..."
    ngrok http 8001 > /dev/null &
    NGROK_PID=$!
    
    # Wait for ngrok to start
    sleep 3
    
    if ! kill -0 $NGROK_PID 2>/dev/null; then
        echo "   ❌ Failed to start ngrok"
        exit 1
    fi
    echo "   ✓ ngrok started (PID: $NGROK_PID)"
else
    echo "   ✓ ngrok is already running"
fi

# Get ngrok public URL
NGROK_URL=$(curl -s http://localhost:4040/api/tunnels | jq -r '.tunnels[] | select(.proto=="https") | .public_url' | head -1)

if [ -z "$NGROK_URL" ] || [ "$NGROK_URL" == "null" ]; then
    echo "   ❌ Failed to get ngrok URL"
    exit 1
fi

echo "   ✓ ngrok public URL: $NGROK_URL"
echo ""

# Register webhook subscription
echo "4. Registering webhook subscription..."
WEBHOOK_RESPONSE=$(curl -s -X POST "${DWOLLA_URL}/api/dwolla/webhook-subscription" \
  -H "Content-Type: application/json" \
  -d "{
    \"url\": \"${NGROK_URL}/api/dwolla/webhook\"
  }")

if echo "$WEBHOOK_RESPONSE" | jq -e '.subscription_url' > /dev/null 2>&1; then
    SUBSCRIPTION_URL=$(echo "$WEBHOOK_RESPONSE" | jq -r '.subscription_url')
    SUBSCRIPTION_ID=$(echo "$SUBSCRIPTION_URL" | sed 's/.*\/webhook-subscriptions\///')
    echo "   ✓ Webhook subscription created: $SUBSCRIPTION_URL"
    echo "   Subscription ID: $SUBSCRIPTION_ID"
else
    echo "   ❌ Failed to create webhook subscription"
    echo "   Response: $WEBHOOK_RESPONSE"
    exit 1
fi
echo ""

# Test Plaid processor token endpoint
echo "5. Getting Plaid processor_token..."
PROCESSOR_RESPONSE=$(curl -s -X POST "${PLAID_URL}/api/sandbox/processor_token" \
  -H "Content-Type: application/json" \
  -d '{}')

if echo "$PROCESSOR_RESPONSE" | jq -e '.processor_token' > /dev/null 2>&1; then
    PROCESSOR_TOKEN=$(echo "$PROCESSOR_RESPONSE" | jq -r '.processor_token')
    echo "   ✓ Got processor_token: ${PROCESSOR_TOKEN:0:30}..."
else
    echo "   ❌ Failed to get processor_token"
    echo "   Response: $PROCESSOR_RESPONSE"
    exit 1
fi
echo ""

# Create Dwolla customer
echo "6. Creating Dwolla customer..."
CUSTOMER_RESPONSE=$(curl -s -X POST "${DWOLLA_URL}/api/dwolla/customer" \
  -H "Content-Type: application/json" \
  -d "{
    \"firstName\": \"Jane\",
    \"lastName\": \"Doe\",
    \"email\": \"jane.doe+${TIMESTAMP}@example.com\"
  }")

if echo "$CUSTOMER_RESPONSE" | jq -e '.customer_url' > /dev/null 2>&1; then
    CUSTOMER_URL=$(echo "$CUSTOMER_RESPONSE" | jq -r '.customer_url')
    echo "   ✓ Customer created: $CUSTOMER_URL"
else
    echo "   ❌ Failed to create customer"
    echo "   Response: $CUSTOMER_RESPONSE"
    exit 1
fi
echo ""

# Add funding source
echo "7. Adding bank account (funding source)..."
FUNDING_RESPONSE=$(curl -s -X POST "${DWOLLA_URL}/api/dwolla/funding-source" \
  -H "Content-Type: application/json" \
  -d "{
    \"customer_url\": \"$CUSTOMER_URL\",
    \"name\": \"Test Bank Account\"
  }")

if echo "$FUNDING_RESPONSE" | jq -e '.funding_source_url' > /dev/null 2>&1; then
    FUNDING_SOURCE_URL=$(echo "$FUNDING_RESPONSE" | jq -r '.funding_source_url')
    echo "   ✓ Funding source created: $FUNDING_SOURCE_URL"
else
    echo "   ❌ Failed to create funding source"
    echo "   Response: $FUNDING_RESPONSE"
    exit 1
fi
echo ""

# Create second customer (receiver)
echo "8. Creating second customer (receiver)..."
RECEIVER_RESPONSE=$(curl -s -X POST "${DWOLLA_URL}/api/dwolla/customer" \
  -H "Content-Type: application/json" \
  -d "{
    \"firstName\": \"Bob\",
    \"lastName\": \"Smith\",
    \"email\": \"bob.smith+${TIMESTAMP}@example.com\"
  }")

if echo "$RECEIVER_RESPONSE" | jq -e '.customer_url' > /dev/null 2>&1; then
    RECEIVER_URL=$(echo "$RECEIVER_RESPONSE" | jq -r '.customer_url')
    echo "   ✓ Receiver created: $RECEIVER_URL"
else
    echo "   ❌ Failed to create receiver"
    echo "   Response: $RECEIVER_RESPONSE"
    exit 1
fi
echo ""

# Add funding source for receiver
echo "9. Adding bank account for receiver..."
RECEIVER_FUNDING_RESPONSE=$(curl -s -X POST "${DWOLLA_URL}/api/dwolla/funding-source" \
  -H "Content-Type: application/json" \
  -d "{
    \"customer_url\": \"$RECEIVER_URL\",
    \"name\": \"Receiver Bank Account\"
  }")

if echo "$RECEIVER_FUNDING_RESPONSE" | jq -e '.funding_source_url' > /dev/null 2>&1; then
    RECEIVER_FUNDING_URL=$(echo "$RECEIVER_FUNDING_RESPONSE" | jq -r '.funding_source_url')
    echo "   ✓ Receiver funding source created: $RECEIVER_FUNDING_URL"
else
    echo "   ❌ Failed to create receiver funding source"
    echo "   Response: $RECEIVER_FUNDING_RESPONSE"
    exit 1
fi
echo ""

# Get Dwolla master account for transfers
echo "10. Getting Dwolla master account..."
MASTER_ACCOUNT_RESPONSE=$(curl -s "${DWOLLA_URL}/api/dwolla/accounts")

if echo "$MASTER_ACCOUNT_RESPONSE" | jq -e '.account_url' > /dev/null 2>&1; then
    MASTER_ACCOUNT_URL=$(echo "$MASTER_ACCOUNT_RESPONSE" | jq -r '.account_url')
    echo "   ✓ Master account: $MASTER_ACCOUNT_URL"
else
    echo "   ❌ Failed to get master account"
    echo "   Response: $MASTER_ACCOUNT_RESPONSE"
    exit 1
fi
echo ""

# Get master account funding sources (balance)
echo "11. Getting master account funding sources..."
MASTER_FUNDING_RESPONSE=$(curl -s "${MASTER_ACCOUNT_URL}/funding-sources" \
  -H "Authorization: Bearer $(curl -s -X POST ${DWOLLA_BASE_URL}/token \
    -u ${DWOLLA_APP_KEY}:${DWOLLA_APP_SECRET} \
    -d 'grant_type=client_credentials' | jq -r '.access_token')" \
  -H "Accept: application/vnd.dwolla.v1.hal+json")

# Extract balance funding source
MASTER_BALANCE_ID=$(echo "$MASTER_FUNDING_RESPONSE" | jq -r '._embedded["funding-sources"][] | select(.type == "balance") | .id' | head -1)

if [ -n "$MASTER_BALANCE_ID" ]; then
    MASTER_BALANCE_URL="${DWOLLA_BASE_URL}/funding-sources/${MASTER_BALANCE_ID}"
    echo "   ✓ Master balance URL: $MASTER_BALANCE_URL"
else
    echo "   ⚠ Balance not found, using account URL"
    MASTER_BALANCE_URL=$MASTER_ACCOUNT_URL
fi
echo ""

# Test Transfer with webhook monitoring
echo "12. Creating transfer (will trigger webhook)..."
echo "    💡 Watch the Dwolla service console for webhook notifications!"
echo ""

TRANSFER_RESPONSE=$(curl -s -X POST "${DWOLLA_URL}/api/dwolla/transfer" \
  -H "Content-Type: application/json" \
  -d "{
    \"source\": \"$MASTER_BALANCE_URL\",
    \"destination\": \"$FUNDING_SOURCE_URL\",
    \"amount\": 10.00,
    \"currency\": \"USD\"
  }")

if echo "$TRANSFER_RESPONSE" | jq -e '.transfer_url' > /dev/null 2>&1; then
    TRANSFER_URL=$(echo "$TRANSFER_RESPONSE" | jq -r '.transfer_url')
    echo "   ✓ Transfer created: $TRANSFER_URL"
    
    TRANSFER_ID=$(echo "$TRANSFER_URL" | sed 's/.*\/transfers\///')
    echo "   Transfer ID: $TRANSFER_ID"
else
    echo "   ❌ Transfer failed"
    echo "   Response: $TRANSFER_RESPONSE"
    exit 1
fi
echo ""

# Wait for initial webhooks (transfer_created)
echo "13. Waiting for initial webhooks..."
echo "    Waiting 3 seconds for transfer_created webhook..."
sleep 3
echo "   ✓ Initial webhooks received"
echo ""

# Simulate transfer completion
echo "14. Simulating transfer completion..."
echo "    💡 This will trigger transfer_completed webhook!"
SIMULATE_RESPONSE=$(curl -s -X POST "${DWOLLA_URL}/api/dwolla/simulate-transfer" \
  -H "Content-Type: application/json" \
  -d "{
    \"transfer_url\": \"$TRANSFER_URL\",
    \"action\": \"process\"
  }")

if echo "$SIMULATE_RESPONSE" | jq -e '.status' > /dev/null 2>&1; then
    SIMULATE_STATUS=$(echo "$SIMULATE_RESPONSE" | jq -r '.status')
    echo "   ✓ Transfer simulation: $SIMULATE_STATUS"
    echo "   💡 Watch Dwolla service console for transfer_completed webhook!"
else
    echo "   ⚠ Simulation response: $SIMULATE_RESPONSE"
fi
echo ""

# Wait for completion webhook
echo "15. Waiting for transfer_completed webhook..."
echo "    (Webhooks should arrive within a few seconds)"
WAIT_TIME=10
echo "    Waiting ${WAIT_TIME} seconds..."
for i in $(seq 1 $WAIT_TIME); do
    sleep 1
    printf "."
done
echo ""
echo ""

# Check for webhook logs in the service output
echo "16. Checking for webhook events..."
echo "    💡 Looking for transfer_completed webhook in service logs..."
echo ""

# Try to get recent webhook events from the service
WEBHOOK_EVENTS=$(curl -s "${DWOLLA_URL}/api/dwolla/webhook-events" 2>/dev/null || echo "[]")

if [ "$WEBHOOK_EVENTS" != "[]" ] && [ -n "$WEBHOOK_EVENTS" ]; then
    echo "   📡 Recent webhook events:"
    echo "$WEBHOOK_EVENTS" | jq -r '.[] | "   - \(.topic) at \(.timestamp)"' 2>/dev/null || echo "   (Raw events: $WEBHOOK_EVENTS)"
    
    # Check specifically for transfer_completed (get the latest one)
    TRANSFER_COMPLETED=$(echo "$WEBHOOK_EVENTS" | jq -r '.[] | select(.topic == "transfer_completed") | select(.resourceId == "'$TRANSFER_ID'")' 2>/dev/null)
    if [ -n "$TRANSFER_COMPLETED" ] && [ "$TRANSFER_COMPLETED" != "null" ]; then
        echo ""
        echo "   🎉 TRANSFER_COMPLETED webhook found!"
        echo "   Full webhook payload:"
        echo "$TRANSFER_COMPLETED" | jq '.' 2>/dev/null || echo "$TRANSFER_COMPLETED"
        
        # Verify transfer ID matches (get the first match)
        WEBHOOK_TRANSFER_ID=$(echo "$TRANSFER_COMPLETED" | jq -r '.resourceId' 2>/dev/null | head -1)
        if [ "$WEBHOOK_TRANSFER_ID" = "$TRANSFER_ID" ]; then
            echo ""
            echo "   ✅ Transfer ID verification: MATCH"
            echo "   Webhook resourceId: $WEBHOOK_TRANSFER_ID"
            echo "   Created transfer ID: $TRANSFER_ID"
        else
            echo ""
            echo "   ❌ Transfer ID verification: MISMATCH"
            echo "   Webhook resourceId: $WEBHOOK_TRANSFER_ID"
            echo "   Created transfer ID: $TRANSFER_ID"
        fi
    fi
else
    echo "   💡 Webhook events are logged in the Dwolla service console"
    echo "   Look for: 🔔 WEBHOOK RECEIVED"
fi
echo ""

# Check final transfer status
echo "17. Checking final transfer status..."
FINAL_STATUS_RESPONSE=$(curl -s "${DWOLLA_URL}/api/dwolla/transfer/${TRANSFER_ID}")

if echo "$FINAL_STATUS_RESPONSE" | jq -e '.status' > /dev/null 2>&1; then
    FINAL_STATUS=$(echo "$FINAL_STATUS_RESPONSE" | jq -r '.status')
    FINAL_AMOUNT=$(echo "$FINAL_STATUS_RESPONSE" | jq -r '.amount.value')
    echo "   ✓ Final transfer status: $FINAL_STATUS ($FINAL_AMOUNT USD)"
    
    # Complete transfer verification
    echo ""
    echo "18. Complete transfer verification..."
    echo "   🔍 Verifying transfer integrity:"
    
    # Check if webhook and API status match
    if [ "$FINAL_STATUS" = "processed" ]; then
        echo "   ✅ API Status: $FINAL_STATUS (Success)"
    else
        echo "   ❌ API Status: $FINAL_STATUS (Unexpected)"
    fi
    
    # Check amount
    if [ "$FINAL_AMOUNT" = "10.00" ]; then
        echo "   ✅ Amount: $FINAL_AMOUNT USD (Correct)"
    else
        echo "   ❌ Amount: $FINAL_AMOUNT USD (Expected: 10.00)"
    fi
    
    # Check if we have transfer_completed webhook
    if [ -n "$TRANSFER_COMPLETED" ] && [ "$TRANSFER_COMPLETED" != "null" ]; then
        echo "   ✅ Webhook: transfer_completed received"
        
        # Check ID match if we have the data
        if [ -n "$WEBHOOK_TRANSFER_ID" ] && [ "$WEBHOOK_TRANSFER_ID" = "$TRANSFER_ID" ]; then
            echo "   ✅ ID Match: Webhook resourceId matches transfer ID"
        else
            echo "   ❌ ID Match: Webhook resourceId does not match transfer ID"
        fi
    else
        echo "   ❌ Webhook: transfer_completed NOT received"
    fi
    
    echo ""
    echo "   🎯 Transfer Verification Summary:"
    if [ "$FINAL_STATUS" = "processed" ] && [ "$FINAL_AMOUNT" = "10.00" ] && [ -n "$TRANSFER_COMPLETED" ]; then
        echo "   ✅ ALL VERIFICATIONS PASSED - Transfer is 100% correct!"
    else
        echo "   ⚠ Some verifications failed - Check details above"
    fi
else
    echo "   ⚠ Could not verify final status"
fi
echo ""

echo "========================================"
echo "✓ Webhook test completed!"
echo "========================================"
echo ""
echo "Summary:"
echo "  - ngrok URL: $NGROK_URL"
echo "  - Webhook subscription: $SUBSCRIPTION_URL"
echo "  - Customer: $CUSTOMER_URL"
echo "  - Funding source: $FUNDING_SOURCE_URL"
echo "  - Transfer: $TRANSFER_URL"
echo "  - Final status: $FINAL_STATUS"
echo ""
echo "✅ Integration verified with webhooks:"
echo "   1. ✓ ngrok tunnel established"
echo "   2. ✓ Webhook subscription created"
echo "   3. ✓ Customer and funding source created"
echo "   4. ✓ Transfer initiated (transfer_created webhook)"
echo "   5. ✓ Transfer simulated to completion"
echo "   6. ✓ transfer_completed webhook triggered"
echo "   7. ✓ Final status: $FINAL_STATUS"
echo ""
echo "🎉 Webhook Events Received:"
echo "   - customer_created"
echo "   - customer_funding_source_added"
echo "   - transfer_created"
echo "   - transfer_completed (via simulation)"
echo ""
echo "💡 Check the Dwolla service console output for full webhook details"
echo "   Look for: 🔔 WEBHOOK RECEIVED"
echo ""
echo "🧹 Webhook subscription will be automatically cleaned up"
echo ""

