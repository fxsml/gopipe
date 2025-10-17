## Workshop: Building Go Pipelines from Scratch

**Goal**: Build and understand the core patterns behind modern Go data pipelines by implementing key components step by step.

**Challenge**: Create a complete pipeline system for batch data processing.

### 1. Basic Pipeline Foundation

- Build a **Producer** that sends numbers 1-N into a channel
- Build a **Consumer** that reads from a channel and prints to stdout
- Understand the fundamental producer-consumer pattern

### 2. Generic Pipeline Components

- Refactor producer and consumer into reusable functions
- Add generic type support for any data type
- Learn how generics enable type-safe pipeline building

### 3. Transform Stage

- Create a pipeline stage that transforms data and forwards it
- Accept a `TransformFunc` for flexible transformations
- Example: Double all numbers passing through

### 4. Collect Stage

- Accumulate data over time or until reaching a size limit
- Forward collected data as batches (slices)
- Configure with `maxDuration` and `maxSize` parameters

### 5. Split Stage

- Take batched data (slices) and emit individual elements
- Complete the batch processing cycle

### 6. Batch Processor

- Combine Collect, Transform, and Split into a unified batch processor
- Accept a `BatchFunc` for processing entire batches
- Handle batch-level operations efficiently

### 7. Concurrent Transform

- Add concurrency support to the Transform stage
- Accept worker count parameter for parallel processing
- Use `sync.WaitGroup` for proper goroutine lifecycle management

### 8. Context-Aware Processing with Error Handling

- Extend Transform with error handling capabilities
- Make `TransformFunc` return `(result, error)`
- Add context cancellation support
- Implement graceful shutdown: drain remaining items through error handler when `ctx.Done()` fires
