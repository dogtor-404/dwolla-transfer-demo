# Dwolla Transfer Demo

This demo shows how to integrate Plaid + Dwolla for bank transfers using pure backend APIs (no frontend UI required).

## Architecture

### Two-Service Design

**plaid-quickstart (Port 8000)**: Plaid Service
- Existing: Real user flow with `/api/create_processor_token`
- New: Sandbox test endpoints for backend-only testing

**dwolla-transfer-demo (Port 8001)**: Dwolla Service
- Pure Dwolla business logic
- Calls Plaid service APIs to get processor_token
- Complete transfer workflow

## Implementation Status

### ✅ Steps 1-2: Plaid Integration (Completed & Tested)

Added Sandbox endpoints to `plaid-quickstart/go/server.go`:

1. **POST /api/sandbox/public_token**: Create sandbox public_token without UI
2. **POST /api/sandbox/processor_token**: Get processor_token in one call (all-in-one)
3. **POST /api/create_processor_token**: Real user flow (requires frontend Plaid Link)

**✅ Tested & Working**: All endpoints successfully tested with Dwolla integration enabled.

### ✅ Steps 3-6: Dwolla Integration (Completed & Tested)

Created `dwolla-transfer-demo` service with endpoints:

3. **POST /api/dwolla/customer**: Create Dwolla customer ✅ Tested
4. **POST /api/dwolla/funding-source**: Add bank account (auto-fetches processor_token from Plaid) ✅ Tested
5. **POST /api/dwolla/transfer**: Initiate transfer ✅ Ready
6. **GET /api/dwolla/transfer/:id**: Check transfer status ✅ Ready

**✅ Full Integration Test Passed**: Complete workflow from Plaid processor_token to Dwolla funding source creation.

## Quick Start

### Prerequisites

1. **Plaid credentials** in `plaid-quickstart/.env`
   - ✅ Dwolla integration must be enabled in Plaid Dashboard
   - Visit: https://dashboard.plaid.com/developers/integrations

2. **Dwolla credentials** in `dwolla-transfer-demo/.env`
   - Sandbox API keys from: https://dashboard-sandbox.dwolla.com/applications-legacy

### Start Both Services

**Terminal 1** - Plaid Service:
```bash
cd plaid-quickstart/go
go run server.go  # Port 8000
```

**Terminal 2** - Dwolla Service:
```bash
cd dwolla-transfer-demo
go run server.go  # Port 8001
```

### Complete Backend Flow (No UI Required!)

**Step 1: Create Dwolla Customer**
```bash
curl -X POST http://localhost:8001/api/dwolla/customer \
  -H "Content-Type: application/json" \
  -d '{
    "firstName": "John",
    "lastName": "Doe",
    "email": "john.doe@example.com"
  }'
```

Response:
```json
{
  "customer_url": "https://api-sandbox.dwolla.com/customers/xxx",
  "status": "created"
}
```

**Step 2: Add Bank Account (Auto-fetches Plaid processor_token)**
```bash
curl -X POST http://localhost:8001/api/dwolla/funding-source \
  -H "Content-Type: application/json" \
  -d '{
    "customer_url": "https://api-sandbox.dwolla.com/customers/xxx",
    "name": "My Bank Account"
  }'
```

This will:
- Automatically call Plaid's `/api/sandbox/processor_token`
- Create sandbox bank account in Plaid (First Platypus Bank)
- Link it to Dwolla customer

Response:
```json
{
  "funding_source_url": "https://api-sandbox.dwolla.com/funding-sources/yyy",
  "status": "created"
}
```

**✅ Tested Example**:
```json
{
  "funding_source_url": "https://api-sandbox.dwolla.com/funding-sources/73c76f3c-1706-4357-97e8-9d3afbad5bd2",
  "status": "created"
}
```

**Step 3: Execute Transfer**
```bash
curl -X POST http://localhost:8001/api/dwolla/transfer \
  -H "Content-Type: application/json" \
  -d '{
    "source": "https://api-sandbox.dwolla.com/funding-sources/source-id",
    "destination": "https://api-sandbox.dwolla.com/funding-sources/destination-id",
    "amount": 10.00,
    "currency": "USD"
  }'
```

**Step 4: Check Transfer Status**
```bash
curl http://localhost:8001/api/dwolla/transfer/transfer-id
```

## API Endpoints

### Plaid Service (Port 8000)

**Sandbox Endpoints (No UI)**:
- `POST /api/sandbox/public_token` - Create sandbox public_token
- `POST /api/sandbox/processor_token` - Get processor_token in one call

**Real User Flow**:
- `POST /api/create_processor_token` - Requires frontend Plaid Link authorization first

### Dwolla Service (Port 8001)

**Core Endpoints**:
- `GET /health` - Health check
- `POST /api/dwolla/customer` - Create Dwolla customer
- `POST /api/dwolla/funding-source` - Link bank account via Plaid
- `POST /api/dwolla/transfer` - Initiate transfer
- `GET /api/dwolla/transfer/:id` - Get transfer status

**Webhook Endpoints**:
- `POST /api/dwolla/webhook-subscription` - Create webhook subscription
- `GET /api/dwolla/webhook-subscriptions` - List webhook subscriptions
- `DELETE /api/dwolla/webhook-subscription/:id` - Delete webhook subscription
- `POST /api/dwolla/webhook` - Receive webhook notifications (called by Dwolla)

## Implementation Details

### Plaid Sandbox Flow

The `/api/sandbox/processor_token` endpoint performs all steps automatically:
1. Creates sandbox public_token (using `ins_109508` - First Platypus Bank)
2. Exchanges for access_token
3. Gets first bank account
4. Creates processor_token for Dwolla

### Dwolla Integration

The `dwolla-transfer-demo` service:
- Authenticates with Dwolla API using OAuth client credentials
- Calls Plaid service to get processor_token
- Uses processor_token to create funding source in Dwolla
- Manages complete transfer workflow

### Why This Architecture?

- ✅ **Service Separation**: Plaid and Dwolla concerns are isolated
- ✅ **Pure Backend**: No frontend UI required for testing
- ✅ **Flexible**: Supports both sandbox testing and real user flows
- ✅ **Independent**: Each service can be developed and tested separately

## Environment Variables

### plaid-quickstart/.env
```bash
PLAID_CLIENT_ID=your_client_id
PLAID_SECRET=your_sandbox_secret
PLAID_ENV=sandbox
```

### dwolla-transfer-demo/.env
```bash
DWOLLA_APP_KEY=your_key
DWOLLA_APP_SECRET=your_secret
DWOLLA_ENV=sandbox
DWOLLA_BASE_URL=https://api-sandbox.dwolla.com
PLAID_API_URL=http://localhost:8000
APP_PORT=8001

# Webhook Configuration (optional, for webhook testing)
DWOLLA_WEBHOOK_SECRET=your_webhook_secret_here
WEBHOOK_BASE_URL=https://your-ngrok-url.ngrok.io
```

See `env.example` for a complete template.

## Webhook Integration

### Overview

Dwolla webhooks provide real-time notifications for events like transfer completion, eliminating the need to poll for status updates. This implementation includes:

- ✅ Automatic webhook signature verification
- ✅ Real-time event logging to console
- ✅ Support for all Dwolla event types
- ✅ ngrok integration for local testing

### Setup Webhooks

#### 1. Install ngrok

**macOS**:
```bash
brew install ngrok/ngrok/ngrok
```

**Linux**:
```bash
snap install ngrok
```

**Windows**: Download from https://ngrok.com/download

#### 2. Configure ngrok

1. Sign up at https://dashboard.ngrok.com/signup
2. Get your authtoken from https://dashboard.ngrok.com/get-started/your-authtoken
3. Configure:
```bash
ngrok config add-authtoken YOUR_AUTHTOKEN
```

#### 3. Generate Webhook Secret

Generate a secure random string for webhook verification:
```bash
openssl rand -base64 32
```

#### 4. Update .env File

Add to your `dwolla-transfer-demo/.env`:
```bash
DWOLLA_WEBHOOK_SECRET=your_generated_secret_here
WEBHOOK_BASE_URL=https://your-ngrok-url.ngrok.io
```

### Using Webhooks

#### Start ngrok Tunnel

In a separate terminal:
```bash
ngrok http 8001
```

Copy the HTTPS URL (e.g., `https://abc123.ngrok.io`) and update `WEBHOOK_BASE_URL` in your `.env` file.

#### Register Webhook Subscription

```bash
curl -X POST http://localhost:8001/api/dwolla/webhook-subscription \
  -H "Content-Type: application/json" \
  -d '{}'
```

The service will automatically use `WEBHOOK_BASE_URL` and `DWOLLA_WEBHOOK_SECRET` from your environment.

Response:
```json
{
  "subscription_url": "https://api-sandbox.dwolla.com/webhook-subscriptions/xxx",
  "webhook_url": "https://abc123.ngrok.io/api/dwolla/webhook",
  "status": "created"
}
```

#### Monitor Webhooks

When transfers or other events occur, you'll see real-time notifications in your Dwolla service console:

```
============================================================
🔔 WEBHOOK RECEIVED at 2024-01-15 10:30:45
============================================================
Event ID:  abc-123-def
Topic:     transfer_completed
Timestamp: 2024-01-15T10:30:45.000Z
Resource:  https://api-sandbox.dwolla.com/transfers/xxx
✅ Transfer completed successfully!

Full webhook payload:
{
  "id": "abc-123-def",
  "topic": "transfer_completed",
  "timestamp": "2024-01-15T10:30:45.000Z",
  ...
}
============================================================
```

#### Supported Event Types

The webhook handler recognizes and logs:
- `transfer_completed` - ✅ Transfer successful
- `transfer_failed` - ❌ Transfer failed
- `transfer_cancelled` - ⚠ Transfer cancelled
- `customer_created` - 👤 Customer created
- `customer_funding_source_added` - 🏦 Funding source added
- `customer_funding_source_verified` - ✓ Funding source verified
- And many more...

### List Active Webhooks

```bash
curl http://localhost:8001/api/dwolla/webhook-subscriptions
```

### Delete Webhook Subscription

```bash
curl -X DELETE http://localhost:8001/api/dwolla/webhook-subscription/SUBSCRIPTION_ID
```

## Testing

### Automated Webhook Test

Run the complete webhook integration test with automatic ngrok setup:
```bash
cd dwolla-transfer-demo
./test_webhook.sh
```

This script will:
1. ✅ Check if ngrok is installed
2. ✅ Start ngrok tunnel automatically
3. ✅ Register webhook subscription
4. ✅ Create customers and funding sources
5. ✅ Execute a transfer
6. ✅ Wait for webhook notifications
7. ✅ Clean up (delete subscription, stop ngrok)

**Expected Output**:
```
========================================
Dwolla Webhook Integration Test
========================================

1. Checking ngrok installation...
   ✓ ngrok is installed

2. Checking services...
   ✓ Plaid service is running
   ✓ Dwolla service is running

3. Setting up ngrok tunnel...
   ✓ ngrok public URL: https://abc123.ngrok.io

4. Registering webhook subscription...
   ✓ Webhook subscription created

...

✅ Integration verified with webhooks
💡 Check server console for webhook notifications
```

### Automated Test Script (Without Webhooks)

Run the complete integration test:
```bash
cd dwolla-transfer-demo
./test_flow.sh
```

**Expected Output**:
```
========================================
Plaid + Dwolla Integration Test
========================================

1. Checking services...
   ✓ Plaid service is running
   ✓ Dwolla service is running

2. Testing Plaid processor_token endpoint...
   ✓ Got processor_token: processor-sandbox-xxx...

3. Creating Dwolla customer...
   ✓ Customer created: https://api-sandbox.dwolla.com/customers/xxx

4. Adding bank account (funding source)...
   ✓ Funding source created: https://api-sandbox.dwolla.com/funding-sources/xxx

========================================
✓ All tests passed!
========================================
```

### Manual Testing

1. **Health Check**: 
   ```bash
   curl http://localhost:8000/health  # Plaid
   curl http://localhost:8001/health  # Dwolla
   ```

2. **Test Plaid Only**:
   ```bash
   curl -X POST http://localhost:8000/api/sandbox/processor_token
   ```

3. **Full Integration**: Follow the Complete Backend Flow above

## Current Status

### ✅ Fully Working & Tested

- **Plaid Sandbox Integration**: ✅ Working with Dwolla integration enabled
- **Dwolla Customer Creation**: ✅ Tested successfully
- **Bank Account Linking**: ✅ Auto-fetches processor_token from Plaid
- **Service Communication**: ✅ Plaid ↔ Dwolla API calls working
- **Transfer Execution**: ✅ Payout and Payin transfers working
- **Webhook Integration**: ✅ Real-time notifications with signature verification
- **Automated Testing**: ✅ Complete test scripts available (with and without webhooks)

### 🚀 Ready for Production

The integration is now ready for:
- Real Dwolla transfers (with proper funding sources)
- Production deployment
- Frontend integration
- Real-time webhook notifications

## References

- [Plaid Sandbox API](https://plaid.com/docs/api/sandbox/)
- [Plaid Processor Token](https://plaid.com/docs/api/processors/)
- [Dwolla API](https://developers.dwolla.com/api-reference)
- [Dwolla + Plaid Integration](https://developers.dwolla.com/docs/open-banking/plaid)
- [Dwolla Webhooks](https://developers.dwolla.com/docs/balance/webhooks)
- [Enable Dwolla Integration in Plaid](https://dashboard.plaid.com/developers/integrations)
- [ngrok Documentation](https://ngrok.com/docs)
