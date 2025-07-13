# Gmail API Emulator

A Docker-based Gmail API emulator that serves transformed Enron email data for development and testing. Provides a drop-in replacement for the real Gmail API without requiring authentication or quotas. Distributed under the Apache 2.0 License.

Developed by Carson Sweet with assistance from Claude Opus 4. 
https://www.carsonsweet.com



## Quick Start

### Option 1: Build from Source

Both the source repo and the Docker image already have 5,000 messages transformed and ready to go. Details on transforming more messages are in the sections below.

```bash
# Clone the repository
git clone https://github.com/yourusername/enron-gmail-emulator.git
cd enron-gmail-emulator

# Start the Gmail API emulator
docker-compose up -d gmail-emulator

# Test it
curl http://localhost:8080/health # simple proof of life
curl http://localhost:8080/ # all available endpoints
curl http://localhost:8080/debug/users # all Gmail users in current dataset
curl http://localhost:8080/gmail/v1/users/me/profile # "me" user profile

```



### Option 2: Use Pre-built Image

```bash
# Pull the pre-built image with transformed data
docker pull ghcr.io/carson-sweet/enron-gmail-emulator:latest

# Run the emulator
docker run -d -p 8080:8080 ghcr.io/yourusername/enron-gmail-emulator:latest

# Test it
curl http://localhost:8080/health # simple proof of life
curl http://localhost:8080/ # all available endpoints
curl http://localhost:8080/debug/users # all Gmail users in current dataset
curl http://localhost:8080/gmail/v1/users/me/profile # "me" user profile

```



## Using the Emulator

Simple. Point your Gmail API client to `http://localhost:8080`:

```python
# Python example
service = build('gmail', 'v1', 
    discoveryServiceUrl='http://localhost:8080/discovery/v1/apis/gmail/v1/rest')
```



```javascript
// Node.js example
const gmail = google.gmail({
  version: 'v1',
  baseURL: 'http://localhost:8080/gmail/v1'
});
```



# Features

### Features

- **5000 preloaded emails** from the Enron corpus with authentic communication patterns
- **Gmail API compatible** - Point your app to `http://localhost:8080` instead of Gmail
- **No authentication required** - Skip OAuth, just start developing and testing
- **Realistic test personas** - Top contacts mapped to consistent identities (Sarah Chen, David Kumar, etc.)
- **Preserved email threads** - Reconstructed from subject lines and participants
- **Time-shifted dates** - Emails appear 3 years old instead of from 2001-2002
- **Label inference** - INBOX, SENT, IMPORTANT, CATEGORY_PERSONAL, etc.
- **Query support** - Search by from:, to:, subject:, after:, before: (not all query parameters are supported )
- **Pagination** - List messages with pageToken support
- **Health endpoints** - `/health` and `/debug/stats` for monitoring 
- **Docker deployment** - Single container or docker-compose setup



### Implemented Gmail API Endpoints

- `GET /gmail/v1/users/{userId}/profile` - User profile
- `GET /gmail/v1/users/{userId}/labels` - List labels
- `GET /gmail/v1/users/{userId}/messages` - List messages with search
- `GET /gmail/v1/users/{userId}/messages/{id}` - Get single message
- `POST /gmail/v1/users/{userId}/messages/batchGet` - Batch get messages
- `POST /oauth2/v4/token` - Mock OAuth (returns fake tokens)



### Omitted Gmail API Functions

This tool was designed as a source for testing or experiementing with ingestion of large numbers of real-world emails—not sending, CRUD'ding, or otherwise manipulating Gmail messages. It's good for testing email reading/analysis applications but not suitable for testing email management features. Specific Gmail API functionsnot implemented include:

- **Sending emails** - No compose, send, or draft functionality
- **Modifying messages** - No update, trash, untrash, or modify labels
- **Threads endpoint** - Messages have threadIds but no thread operations
- **Attachments** - No attachment download or metadata
- **History API** - No sync or history.list functionality
- **Watch/Push** - No push notifications or watch setup
- **Filters** - No filter management
- **Settings** - No settings, forwarding, or vacation responders
- **Complex queries** - No support for has:attachment, is:unread, etc.
- **Batch operations** - Only batchGet implemented, no batch modify/delete

If you want to contribute, fork on or contact me for contributor access.



# Detailed Setup

This guide provides instructions for creating a Docker container that emulates the Gmail API using transformed Enron email data. The container can be used as a drop-in replacement for the real Gmail API during development and testing. This guide assumes you will create a project directory called `enron-gmail-emulator` and all file paths are relative to this directory unless otherwise specified.



## Prerequisites

1. **Enron Email Dataset**: Download from [here](https://www.cs.cmu.edu/~enron/) or Kaggle and extract the archive. You should see a large set of directories with users, messages, etc. in an extracted folder called `maildir`.  Both the source and the prebuilt container already have ~5,000 messages to get you started, but you will need the Enron dataset to go further.
2. **Docker**: For running the emulator container
3. **Go 1.21+**: For building the transformer (or use the pre-built Docker image)
   
   

## Option 1: Build from Source

#### 1. Clone the repository

```bash
git clone https://github.com/carson-sweet/enron-gmail-emulator.git
cd enron-gmail-emulator
```



#### 2. Move the downloaded Enron dataset in data/maildir/

```bash
mv <download_dir>/maildir/* ./data/maildir/
```

You should now see directories like: data/maildir/kaminski-v/, data/maildir/dasovich-j/, etc.



#### 3. Transform the Enron data into Gmail format (optional)

This step is optional because there are already approximately 5,000 messages already transformed and ready to go. If you need more messages or want to do something differently (like change the "me" user from the perspective of Gmail API), see the full transformer documentaton in later sections.

```bash
docker-compose --profile transform run transformer
```



#### 4. Run the emulator

This will take a minute while Docker builds the container for the first time. If you're running other services, you might need to adjust the `docker-compose.yml` in the base project directory to set ports used, etc.

```bash
docker-compose up -d gmail-emulator
```



#### 5. Test it out

```bash
curl http://localhost:8080/health # simple proof of life
curl http://localhost:8080/ # all available endpoints
curl http://localhost:8080/debug/users # all Gmail users in current dataset
curl http://localhost:8080/gmail/v1/users/me/profile # "me" user profile
```





## Option 2: Use Pre-built Image

#### 1. Pull the pre-built image with transformed data

```bash
docker pull ghcr.io/carson-sweet/enron-gmail-emulator:latest
```



#### 2. Run the emulator

```bash
docker run -d -p 8080:8080 ghcr.io/yourusername/enron-gmail-emulator:latest
```



#### 3. Test it out

```bash
curl http://localhost:8080/health # simple proof of life
curl http://localhost:8080/ # all available endpoints
curl http://localhost:8080/debug/users # all Gmail users in current dataset
curl http://localhost:8080/gmail/v1/users/me/profile # "me" user profile
```



# Using The Emulator



## For Google API Client Libraries

Most Google client libraries support custom endpoints that allow you to use an emulator. 

**Python:**

```python
from googleapiclient.discovery import build
from googleapiclient.http import build_http

# Create custom http object that doesn't validate SSL
http = build_http()

# Build service with custom endpoint
service = build('gmail', 'v1', 
    http=http,
    discoveryServiceUrl='http://localhost:8080/discovery/v1/apis/gmail/v1/rest')
```



**Node.js:**

```javascript
const {google} = require('googleapis');

const gmail = google.gmail({
  version: 'v1',
  baseURL: 'http://localhost:8080/gmail/v1'
});
```



**Go:**

```go
import (
    "google.golang.org/api/gmail/v1"
    "google.golang.org/api/option"
)

service, err := gmail.NewService(ctx,
    option.WithEndpoint("http://localhost:8080/gmail/v1"),
    option.WithoutAuthentication(),
)
```



## Using Environment Variables

Set environment variables to redirect Gmail API calls:

```bash
# For your application
export GMAIL_API_ENDPOINT=http://localhost:8080/gmail/v1
export GMAIL_API_KEY=dummy-key-for-emulator

# Run your application
./your-app
```



## Basic Functions

These are curl commands to keep the demonstration of the various endpoints simple. 

### Basic Health Check

```bash
curl http://localhost:8080/health
```



### List Messages for "Me" User

```bash
curl http://localhost:8080/gmail/v1/users/me/messages
```



### Search Messages

```bash
# By sender
curl "http://localhost:8080/gmail/v1/users/me/messages?q=from:sarah.chen@gmail.com"

# By date
curl "http://localhost:8080/gmail/v1/users/me/messages?q=after:2021-01-01"

# By subject
curl "http://localhost:8080/gmail/v1/users/me/messages?q=subject:meeting"
```



### Get Message Details

```bash
# Get message list first
MESSAGES=$(curl -s http://localhost:8080/gmail/v1/users/me/messages)
MSG_ID=$(echo $MESSAGES | jq -r '.messages[0].id')

# Get full message
curl "http://localhost:8080/gmail/v1/users/me/messages/$MSG_ID"
```



## Available Endpoints

The emulator implements these Gmail API v1 endpoints:

| Endpoint                                     | Method | Description                 |
| -------------------------------------------- | ------ | --------------------------- |
| `/gmail/v1/users/{userId}/profile`           | GET    | Get user profile            |
| `/gmail/v1/users/{userId}/labels`            | GET    | List labels                 |
| `/gmail/v1/users/{userId}/messages`          | GET    | List messages (with search) |
| `/gmail/v1/users/{userId}/messages/{id}`     | GET    | Get message                 |
| `/gmail/v1/users/{userId}/messages/batchGet` | POST   | Batch get messages          |
| `/health`                                    | GET    | Health check                |
| `/debug/stats`                               | GET    | Debug statistics            |



## Query Support

The emulator supports these Gmail search operators:

- `from:email` - Messages from specific sender
- `to:email` - Messages to specific recipient  
- `subject:text` - Messages with text in subject
- `after:YYYY-MM-DD` - Messages after date
- `before:YYYY-MM-DD` - Messages before date
- Plain text search in subject and snippet



## Email Data Customization with Transformer

### Change the Test User

The Gmail API always takes the perspective of "me", that is, the user who is logged into the API. If you want to use a different persona from the Enron data, you can do that by re-running the transformer job. 

Modify the transformer command in `enron-gmail-emulator/docker-compose.yml`:

```yaml
command: >
  --enron-path /input
  --output /output
  --user dasovich-j  # Different Enron user for "me" (see data directories)
  --limit 10000      # More emails
```



### Adjust Time Period

The transformer shifts emails from 2001-2002 to 3 years from the current datetime of the transformer job y default. To change this, modify the `baseDate` calculation in `enron-gmail-emulator/transformer/enron_transformer.go`:

```go
// Line ~81: Change from 3 years to 1 year ago
baseDate := time.Now().AddDate(-1, 0, 0)
```



### Add Custom Personas

Edit the `personas` array in `enron-gmail-emulator/transformer/enron_transformer.go` (around line 297) to map Enron contacts to your test personas:

```go
personas := []TestPersona{
    {Name: "Alice Johnson", Email: "alice@family.com", Role: "spouse"},
    {Name: "Bob Smith", Email: "bob@techcorp.com", Role: "cto"},
    // Add your custom personas
}
```



## Basic Troubleshooting

### Container won't start

Check logs from the `enron-gmail-emulator` directory:

```bash
docker-compose logs gmail-emulator
```



### No data available

Verify transformation completed:

```bash
# From enron-gmail-emulator directory
ls -la data/transformed/
# Should contain gmail_messages.json
```



### Search not working

The emulator supports basic Gmail query syntax. Complex queries like `has:attachment` are not implemented.



### Memory issues

For large datasets, increase Docker memory:

```bash
# From enron-gmail-emulator directory
docker run -m 2g -p 8080:8080 gmail-emulator
```



# Summary

This Docker container provides a complete Gmail API emulation using real email communication patterns from the Enron dataset. It requires no changes to your application code - simply point your Gmail API client to `http://localhost:8080` instead of the real Gmail API.

The Enron dataset provides excellent test coverage with:

- Real email threads and conversations
- Natural relationship patterns  
- Business and personal communications
- 5000 messages spanning multiple years
- Realistic communication frequency and patterns

Perfect for:

- Development without Gmail API quotas
- Integration testing with predictable data
- Demo environments
- Offline development
