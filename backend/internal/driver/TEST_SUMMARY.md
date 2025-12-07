# ClaudeDriver Test Summary

## Test Coverage

**Overall Coverage: 61.7%**

### Fully Covered Functions (100%)
- ✅ `NewClaudeDriver` - Driver initialization
- ✅ `Name` - Driver name getter
- ✅ `Parse` - Main parsing function
- ✅ `Reset` - Buffer reset
- ✅ `Flush` - Flush pending messages
- ✅ `stripANSI` - ANSI sequence removal
- ✅ `SendCommand` - Command formatting
- ✅ `SendSlashCommand` - Slash command formatting
- ✅ `SelectMenuItem` - Menu item selection

### Well Covered Functions (>75%)
- ✅ `FormatInput` - 87.5% - Input action formatting
- ✅ `formatQuestionResponse` - 81.2% - Question response formatting
- ✅ `RespondToEvent` - 75.0% - Event response generation

### Partially Covered Functions (50-75%)
- ⚠️ `formatConfirmation` - 60.0% - Confirmation formatting
- ⚠️ `formatClaudeConfirmResponse` - 57.1% - Claude menu response
- ⚠️ `parseMessages` - 57.4% - Message parsing
- ⚠️ `extractPrompt` - 53.8% - Prompt extraction
- ⚠️ `isUINoiseOrLoading` - 52.9% - UI noise filtering

### Low Coverage Functions (<50%)
- ❌ `formatKey` - 33.3% - Key formatting (many key types not tested)
- ❌ `flushOutputBlock` - 14.3% - Output block flushing (complex logic)

## Test Suite

### Test Categories

#### 1. Output Parsing Tests (8 tests)
- ✅ Question pattern detection `(y/n)`, `(yes/no)`
- ✅ Claude menu pattern detection
- ✅ User input detection `> command`
- ✅ Claude action detection `● Write(file)`
- ✅ Action result detection `⎿ result`
- ✅ Multi-line output collection
- ✅ Message deduplication
- ✅ ANSI sequence stripping

#### 2. Input Formatting Tests (6 tests)
- ✅ Text input formatting
- ✅ Command input formatting
- ✅ Key input formatting (enter, esc, ctrl+c, etc.)
- ✅ Confirmation formatting
- ✅ Cancel/interrupt formatting
- ✅ Slash command formatting

#### 3. Event Response Tests (1 test)
- ✅ Question response formatting
- ✅ Claude confirm menu response
- ✅ Different response types (yes/no, 1/2/esc)

#### 4. Utility Tests (4 tests)
- ✅ Driver name
- ✅ Buffer reset
- ✅ Message flushing
- ✅ Buffer size limit

#### 5. Menu Selection Tests (1 test)
- ✅ Direct number selection (1-9)
- ✅ Arrow navigation for larger indices

### Total Tests: 20 test functions with 60+ sub-tests

## Test Examples

### Example 1: Question Pattern Detection
```go
func TestClaudeDriver_Parse_QuestionPattern(t *testing.T) {
    driver := NewClaudeDriver()
    result, _ := driver.Parse([]byte("Continue? (y/n)"))
    
    // Verifies:
    // - SmartEvent is generated
    // - Event kind is "question"
    // - Options are ["y", "n"]
}
```

### Example 2: Input Formatting
```go
func TestClaudeDriver_FormatInput(t *testing.T) {
    driver := NewClaudeDriver()
    
    // Test command input
    result := driver.FormatInput(InputAction{
        Type: "command",
        Content: "hello",
    })
    // Expected: "hello\r"
    
    // Test key input
    result = driver.FormatInput(InputAction{
        Type: "key",
        Content: "ctrl+c",
    })
    // Expected: "\x03"
}
```

### Example 3: Event Response
```go
func TestClaudeDriver_RespondToEvent(t *testing.T) {
    driver := NewClaudeDriver()
    
    event := SmartEvent{
        Kind: "question",
        Options: []string{"y", "n"},
    }
    
    result := driver.RespondToEvent(event, "yes")
    // Expected: "y\r"
}
```

## Running Tests

### Run all tests
```bash
go test ./internal/driver/
```

### Run with verbose output
```bash
go test -v ./internal/driver/
```

### Run specific test
```bash
go test ./internal/driver/ -run TestClaudeDriver_Parse_QuestionPattern
```

### Generate coverage report
```bash
go test ./internal/driver/ -cover -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Run tests with race detection
```bash
go test -race ./internal/driver/
```

## Integration Tests

The test suite focuses on unit tests. For integration testing, see:
- `playground/test_driver_input.go` - Full integration with PTY
- `playground/test_input_v2.go` - Basic input testing
- `playground/test_input_v3.go` - Input clearing testing

## Future Improvements

### Additional Test Coverage Needed

1. **formatKey** - Test all key types:
   - Arrow keys (up, down, left, right)
   - Function keys
   - Special keys (home, end, page up/down)

2. **flushOutputBlock** - Test complex scenarios:
   - Multiple output blocks
   - Nested output structures
   - Edge cases in block detection

3. **parseMessages** - Test more message types:
   - Resume session tracking
   - Interrupted operations
   - Complex multi-line outputs

4. **isUINoiseOrLoading** - Test all UI patterns:
   - Loading indicators
   - Border elements
   - Menu items
   - Navigation hints

### Property-Based Testing

Consider adding property-based tests for:
- ANSI sequence stripping (any valid ANSI should be removed)
- Buffer size management (buffer should never exceed max size)
- Message deduplication (duplicate messages should be filtered)

### Benchmark Tests

Add benchmark tests for:
- Parse performance with large outputs
- Buffer management efficiency
- Pattern matching performance

## Test Maintenance

### When to Update Tests

1. **Adding new message types** - Add corresponding parse tests
2. **Adding new input types** - Add corresponding format tests
3. **Changing patterns** - Update pattern detection tests
4. **Adding new keys** - Add key formatting tests

### Test Naming Convention

- `TestClaudeDriver_<Method>` - Tests for public methods
- `TestClaudeDriver_<Method>_<Scenario>` - Tests for specific scenarios
- Sub-tests use descriptive names: `t.Run("scenario name", ...)`

## Continuous Integration

These tests should be run:
- ✅ On every commit (pre-commit hook)
- ✅ On every pull request
- ✅ Before releases
- ✅ Nightly with race detection

## Test Quality Metrics

- ✅ All tests are deterministic (no flaky tests)
- ✅ Tests are independent (can run in any order)
- ✅ Tests are fast (< 5ms per test)
- ✅ Tests have clear assertions
- ✅ Tests use table-driven approach where appropriate
- ✅ Tests have descriptive names and comments
