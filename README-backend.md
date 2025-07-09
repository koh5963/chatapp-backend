# ChatApp Backend

This is the backend for ChatApp, with both Go (Gin) and Node (Express) implementations.

## Stack

- Go (Gin)
- Node.js (Express)
- PostgreSQL
- Docker

## Getting Started

### Docker

```bash
docker-compose up
```

### Endpoints

- Base URL: `http://localhost:3001/api/v1`
- Example: `GET /api/v1/ping`

## Notes

- You can switch between Go and Node by changing the `docker-compose.yml` service.
- Environment variables are loaded from `.env`.
