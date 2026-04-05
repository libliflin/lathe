Fix things in this order. Never fix a higher layer while a lower one is broken.

```
Layer 0: Compilation          — Does it build? (cargo build)
Layer 1: Tests                — Do tests pass? (cargo test)
Layer 2: Static analysis      — Is it clean? (cargo clippy, no warnings)
Layer 3: Code quality         — Idiomatic Rust? Proper error handling? No unnecessary unsafe?
Layer 4: Architecture         — Good module structure? Clean trait boundaries?
Layer 5: Documentation        — Rustdoc, README, examples
Layer 6: Features             — New functionality, improvements
```
