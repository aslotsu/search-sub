# WARP.md

This file provides guidance to WARP (warp.dev) when working with code in this repository.

## Project Overview

`search-sub` is a Go microservice that maintains search indexes in Typesense by subscribing to NATS events. It acts as an event-driven synchronization service that keeps user and post data in sync between the main application and search infrastructure.

## Architecture

### Core Components

- **SubscriberService**: Main service orchestrating NATS subscriptions and Typesense operations
- **Event Handlers**: Dedicated handlers for each event type (user created/updated/deleted, post upserted/deleted)
- **Document Mappers**: Transform domain models to Typesense-compatible documents

### Data Models

- **User**: Represents user entities with profile information (username, bio, school, program, etc.)
- **Post**: Represents post entities with content, metadata, and embedded user information

### External Dependencies

- **NATS**: Message broker for receiving domain events
- **Typesense**: Search engine with separate collections for users and posts
- **Environment Configuration**: Uses `.env` file for NATS credentials (not tracked in git)

## Development Commands

### Building and Running

```bash
# Build the application
go build -o search-sub .

# Run the application (requires .env with NATS_CREDS)
go run main.go

# Clean and download dependencies
go mod tidy

# Verify dependencies
go mod verify
```

### Development Workflow

```bash
# Format code
go fmt ./...

# Vet code for potential issues
go vet ./...

# Run with race detection (useful for concurrent code)
go run -race main.go
```

## Event Subscriptions

The service subscribes to these NATS subjects:

- `users.created` - Creates new user documents in Typesense
- `users.updated` - Updates existing user documents  
- `users.deleted` - Removes user documents
- `posts.upsert` - Creates or updates post documents
- `posts.deleted` - Removes post documents

## Configuration

### Environment Variables

- `NATS_CREDS`: Complete NATS user credentials (JWT + NKEY) as multiline string

### Typesense Connections

The service connects to two Typesense instances:
- Users collection: `https://users2.exobook.ca:8108`
- Posts collection: `https://posts2.exobook.ca`

## Key Implementation Details

- NATS credentials are written to a temporary file at startup for security
- All Typesense operations use upsert semantics where appropriate
- Error handling includes detailed logging with emoji indicators for visibility
- Document transformation converts time fields to Unix timestamps for Typesense compatibility
- The main goroutine blocks indefinitely (`select {}`) to keep the service running

## Testing Considerations

When adding tests, consider:
- Mocking NATS connections for unit tests
- Testing document transformation functions independently
- Integration tests with test Typesense instances
- Event handler behavior under error conditions