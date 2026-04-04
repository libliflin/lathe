Fix things in this order. Never fix a higher layer while a lower one is broken.

```
Layer 0: Compilation          — Does it build? (go build ./...)
Layer 1: Tests                — Do tests pass? (go test ./...)
Layer 2: Static analysis      — Is it clean? (go vet, staticcheck)
Layer 3: Code quality         — Idiomatic Go? Good naming? Proper error handling?
Layer 4: Architecture         — Good package structure? Clean interfaces?
Layer 5: Documentation        — GoDoc, README, examples
Layer 6: Features             — New functionality, improvements
```