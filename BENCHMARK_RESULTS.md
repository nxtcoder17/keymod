# Keymod Benchmark Results

## Performance Analysis

### Key Mapping Operations

| Benchmark            | Operations/sec  | Time/op   | Memory/op   | Allocations/op   |
| -----------          | --------------- | --------- | ----------- | ---------------- |
| **Tap Behavior**     | 315,159         | 3.76 µs   | 6,461 B     | 35 allocs        |
| **Hold Behavior**    | 161,918         | 7.11 µs   | 12,796 B    | 66 allocs        |
| **Passthrough Keys** | 176,851         | 6.42 µs   | 12,628 B    | 60 allocs        |
| **Complex Sequence** | 78,440          | 14.66 µs  | 25,714 B    | 136 allocs       |

### Low-Level Operations

| Benchmark                  | Operations/sec  | Time/op   | Memory/op   | Allocations/op   |
| -----------                | --------------- | --------- | ----------- | ---------------- |
| **Parse Key Code**         | 9,026,971       | 131.7 ns  | 0 B         | 0 allocs         |
| **Dispatch Single Key**    | 37,656,135      | 32.34 ns  | 24 B        | 1 allocs         |
| **Dispatch Multiple Keys** | 32,302,251      | 37.17 ns  | 24 B        | 1 allocs         |
| **Config Lookup**          | 16,750,678      | 76.42 ns  | 0 B         | 0 allocs         |

## Key Findings

1. **Tap behavior** is the most efficient mapping operation at ~3.76 µs per operation
2. **Hold behavior** takes roughly 2x longer due to additional state management
3. **Complex sequences** are expectedly slower at ~14.66 µs but still very fast
4. **Key code parsing** and **config lookups** are extremely efficient with zero allocations
5. **Event dispatching** is very fast at ~32-37 ns per operation

## Performance Characteristics

- The key mapper can handle **260,000+ tap events per second** on a single core
- Memory allocations are reasonable, with most coming from string operations in logging
- Configuration lookups using Go's map are very efficient (76 ns)
- The system should have no trouble keeping up with human typing speeds (typically < 1000 keystrokes/minute)

## Optimization Opportunities

1. The main memory allocations come from string operations in event handling
2. Consider pooling InputEvent objects to reduce allocations
3. Pre-compute common key combinations to reduce runtime lookups
