# Go Testing Patterns

## Table-Driven Tests

The standard Go testing pattern. Use for any function with multiple input/output cases:

```go
func TestFoo(t *testing.T) {
    tests := []struct {
        name string
        input string
        want  string
    }{
        {"empty", "", ""},
        {"basic", "hello", "HELLO"},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := Foo(tt.input)
            if got != tt.want {
                t.Errorf("Foo(%q) = %q, want %q", tt.input, got, tt.want)
            }
        })
    }
}
```

## Test Helpers

Use `t.Helper()` for shared assertion functions so failures report the caller's line:

```go
func assertEqual(t *testing.T, got, want interface{}) {
    t.Helper()
    if got != want {
        t.Errorf("got %v, want %v", got, want)
    }
}
```

## Subtests

Use `t.Run` for grouping related tests. Names should describe the scenario, not the function.

## Benchmarks

Add benchmarks for performance-critical code:

```go
func BenchmarkFoo(b *testing.B) {
    for i := 0; i < b.N; i++ {
        Foo("test input")
    }
}
```

## Test Commands

```bash
go test ./...                    # Run all tests
go test ./... -v                 # Verbose output
go test ./... -run TestFoo       # Run specific test
go test ./... -cover             # Show coverage
go test ./... -race              # Race condition detection
go test ./... -count=1           # Disable test caching
```
