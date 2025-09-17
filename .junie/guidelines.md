# Junie Development Guidelines

## Core Principles
- Write production-ready, idiomatic Go code
- Prefer simplicity over cleverness
- Use standard library patterns and interfaces
- Write code that feels natural, not AI-generated
- Add human touches: pragmatic shortcuts, occasional TODOs
- Keep comments concise and focused on "why", not "what"

## Code Style
- Use short variable names (ctx, cfg, db, err)
- Keep functions under 30 lines when possible
- Prefer early returns over nested conditionals
- Use table-driven tests for multiple test cases
- Add blank lines sparingly for logical grouping
- Use receiver abbreviations (2-3 chars max)

## Error Handling
- Create custom error types for domain errors
- Wrap errors with context using fmt.Errorf with %w
- Return structured errors, not just strings
- Use errors.Is and errors.As for error checking
- Add error context at boundaries, not every level

## Concurrency Patterns
- Use context.Context for cancellation and timeouts
- Implement worker pools with buffered channels
- Use sync.Pool for object reuse
- Add proper shutdown with sync.WaitGroup
- Implement back-pressure handling
- Use atomic operations for counters

## Database Patterns
- Implement Repository pattern for data access
- Use transactions with defer rollback pattern
- Create connection pools with proper limits
- Add database health checks
- Use prepared statements for repeated queries
- Implement proper connection lifecycle management

## Architecture Patterns
- Separate domain logic from infrastructure
- Use dependency injection via constructors
- Implement interfaces in consuming packages
- Create aggregate roots for business entities
- Use command/query separation for different operations
- Add event sourcing for audit trails

## Testing Guidelines
- Write integration tests for critical paths
- Use testify for assertions and mocking
- Create test fixtures and helpers
- Test error conditions and edge cases
- Use build tags for integration tests
- Add benchmarks for performance-critical code

## Performance Considerations
- Use profiling with pprof for optimization
- Implement object pooling for high-frequency allocations
- Add caching with proper TTL and memory limits
- Use buffered I/O for file operations
- Implement rate limiting with token buckets
- Add metrics collection with Prometheus patterns

## Documentation
- Write package docs for public APIs
- Add examples in doc comments when helpful
- Create README files for complex packages
- Document configuration options and defaults
- Add architectural decision records (ADRs)
- Keep inline comments minimal and focused

## Project Structure
- Follow standard Go project layout
- Use internal/ for private packages
- Keep cmd/ minimal with business logic in internal/
- Create pkg/ for reusable components
- Add proper go.mod versioning
- Use embedding for static assets

## Security Practices
- Validate all external inputs
- Use prepared statements for SQL
- Implement proper authentication/authorization
- Add rate limiting for public APIs
- Log security events appropriately
- Handle secrets through environment variables

## Monitoring and Observability
- Add structured logging with context
- Implement health checks for all components
- Create metrics for business and technical KPIs
- Add distributed tracing for complex flows
- Use circuit breakers for external dependencies
- Implement graceful degradation patterns

## Human Code Characteristics
- Add occasional TODO or FIXME comments
- Use pragmatic variable names (sometimes abbreviated)
- Leave some debug prints in development code
- Add comments explaining business logic, not obvious code
- Use common Go idioms and patterns
- Make code readable without excessive documentation