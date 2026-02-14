# JWT Authentication Whitelist Configuration

## Overview

The JWT authentication middleware now supports a whitelist mechanism that allows certain URL paths to skip authentication. This provides flexibility in managing authentication requirements across different endpoints.

## Default Whitelist

The following paths are whitelisted by default:

| Path | Methods | Match Type | Description |
|------|---------|------------|-------------|
| `/healthz` | GET | exact | Health check endpoint |
| `/serverless/v1/componentshealth` | GET | exact | Component health check |
| `/client/v1/lease` | PUT, DELETE | exact | Lease management |
| `/client/v1/lease/keepalive` | POST | exact | Lease keepalive |

**Note**: Terminal paths (`/terminal/*`) have been removed from the whitelist and now require JWT authentication.

## Public Functions

Public functions are **not** handled at the HTTP whitelist level. Instead, they are processed by the `FunctionJWTAuthCheck` middleware at the invocation layer, which:

1. Loads function metadata to check if `IsFuncPublic` is true
2. Skips JWT authentication for public functions
3. Applies full JWT validation for private functions

This design separates static HTTP-level URL rules from dynamic function-level business logic.

## Usage

### Global JWT Authentication

The global JWT authentication middleware is automatically applied to all routes:

```go
func InitRoute(r *gin.Engine) {
    // Apply global JWT authentication middleware with whitelist support
    r.Use(middleware.GlobalJWTAuthMiddleware())
    
    // Register your routes here
    r.POST("/api/endpoint", handler)
}
```

### Adding Custom Whitelist Rules

You can add custom whitelist rules at runtime:

```go
import "frontend/pkg/frontend/middleware"

// Add a single rule
middleware.AddAuthWhitelistRule(middleware.AuthWhitelistRule{
    Path:      "/custom/api",
    Methods:   []string{"POST", "GET"},
    MatchType: "exact",
    SkipAuth:  true,
})

// Or set multiple rules at once
middleware.SetAuthWhitelist([]middleware.AuthWhitelistRule{
    {
        Path:      "/public/api",
        Methods:   []string{},  // Empty means all methods
        MatchType: "prefix",
        SkipAuth:  true,
    },
    {
        Path:      "/internal/metrics",
        Methods:   []string{"GET"},
        MatchType: "exact",
        SkipAuth:  true,
    },
})
```

## Whitelist Rule Structure

```go
type AuthWhitelistRule struct {
    Path       string   // URL path pattern
    Methods    []string // HTTP methods (empty means all methods)
    MatchType  string   // "exact" or "prefix"
    SkipAuth   bool     // If true, skip authentication for this path
}
```

### Match Types

- **exact**: The path must exactly match the rule path
  - Example: Path `/healthz` only matches `/healthz`, not `/healthz/status`

- **prefix**: The path must start with the rule path
  - Example: Path `/terminal` matches `/terminal`, `/terminal/ws`, `/terminal/static/style.css`, etc.

### Methods

- If the `Methods` array is empty, the rule applies to all HTTP methods
- Otherwise, the request method must be in the `Methods` array

## Examples

### Example 1: Skip auth for all public API endpoints

```go
middleware.AddAuthWhitelistRule(middleware.AuthWhitelistRule{
    Path:      "/api/public",
    Methods:   []string{},
    MatchType: "prefix",
    SkipAuth:  true,
})
```

This will skip authentication for:
- `/api/public/info`
- `/api/public/status`
- `/api/public/v1/data`

### Example 2: Allow unauthenticated GET requests only

```go
middleware.AddAuthWhitelistRule(middleware.AuthWhitelistRule{
    Path:      "/api/read-only",
    Methods:   []string{"GET"},
    MatchType: "exact",
    SkipAuth:  true,
})
```

This will skip authentication for `GET /api/read-only` but still require authentication for `POST /api/read-only`.

### Example 3: Whitelist multiple specific paths

```go
middleware.SetAuthWhitelist([]middleware.AuthWhitelistRule{
    {
        Path:      "/metrics",
        Methods:   []string{"GET"},
        MatchType: "exact",
        SkipAuth:  true,
    },
    {
        Path:      "/ready",
        Methods:   []string{"GET"},
        MatchType: "exact",
        SkipAuth:  true,
    },
    {
        Path:      "/live",
        Methods:   []string{"GET"},
        MatchType: "exact",
        SkipAuth:  true,
    },
})
```

## Authentication Flow

### HTTP Layer (GlobalJWTAuthMiddleware)

1. Request arrives at the global JWT middleware
2. Check if the request path and method match any whitelist rule
3. If matched and `SkipAuth` is true, skip authentication and continue
4. If not matched, apply JWT authentication:
   - Check if authentication is enabled in config
   - Validate JWT token from X-Auth header
   - Validate role (developer)
   - Validate with IAM server
5. If authentication passes or is skipped, proceed to the handler

### Invocation Layer (FunctionJWTAuthCheck)

For function invocations, an additional layer of authentication applies:

1. Load function metadata
2. Check if function is public (`IsFuncPublic`)
3. If public or auth disabled, skip JWT validation
4. If private, validate:
   - JWT token presence
   - JWT parsing and role extraction
   - Tenant ID validation (for developer role)
   - IAM server validation
5. Proceed with function invocation

## Configuration

JWT authentication can be enabled/disabled in the configuration:

```json
{
  "iamConfig": {
    "enableFuncTokenAuth": true,
    "addr": "http://iam-server:8080"
  }
}
```

- `enableFuncTokenAuth`: Enable/disable JWT authentication globally
- `addr`: IAM server address for token validation

## Testing

The whitelist functionality is fully tested. See `jwtauth_whitelist_test.go` for examples:

```bash
# Run whitelist tests
go test -v frontend/pkg/frontend/middleware -run TestIsInAuthWhitelist
go test -v frontend/pkg/frontend/middleware -run TestGlobalJWTAuthMiddleware
```

## Security Considerations

1. **Review whitelist rules carefully**: Whitelisted paths are accessible without authentication
2. **Use exact match when possible**: Prefix matching is more flexible but requires careful consideration
3. **Limit methods**: If an endpoint only needs GET requests to be public, don't whitelist all methods
4. **Regular audits**: Periodically review whitelist rules to ensure they're still necessary
5. **Custom rules precedence**: Custom rules are checked before default rules, allowing you to override defaults

## Migration from Previous Implementation

Previously, JWT middleware was manually applied to specific routes:

```go
// Old approach
r.POST(urlPreInvoke, middleware.JWTAuthMiddleware(), frontend.InvokeHandler)
```

Now, with global middleware and whitelist:

```go
// New approach - apply once globally
r.Use(middleware.GlobalJWTAuthMiddleware())

// All routes are protected by default
r.POST(urlPreInvoke, frontend.InvokeHandler)

// Specific routes can be whitelisted
middleware.AddAuthWhitelistRule(middleware.AuthWhitelistRule{
    Path:      urlHealthCheck,
    Methods:   []string{"GET"},
    MatchType: "exact",
    SkipAuth:  true,
})
```

This approach provides:
- Centralized authentication management
- Easier to audit which endpoints require authentication
- Reduces risk of forgetting to add authentication to new endpoints
- More flexible configuration options
