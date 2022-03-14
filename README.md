# Java Flight Recorder parser library written in Go.

The parser design is generic, and it should be able to support any kind of type and event.

Current implementation is incomplete, with a focus in supporting the types and events generated by [async-profiler](https://github.com/jvm-profiling-tools/async-profiler). The implementation can be easily extended to support more types and events, see the [Design](#design) section for details.

## Design

While the parser is built on top of the `io.Reader` interface, it doesn't process the input sequentially: the Metadata and Checkpoint events are processed before the rest of the events.
This means that whole Chunks are stored in memory. Chunks are currently processed sequentially, but this is an implementation detail and they may be processed concurrently in the future.

A reader package takes care of the wire-level details, like (un)compressed integers and different string encodings (not all of them are currently supported).

A parser package processes the chunks and returns the events of each of them. In order to do so, it processes the Metadata event and uses that information to parse the rest of the events.
The constant pool is parsed in two passes: in the first pass the inline data is processed and the recursive constant pool references are left unprocessed. On the second pass the constant pool references are resolved.

Finally, the rest of events are processed, using the resolved constant pool data when constant pool references appear.

The parser relies on finding an implementation for each of the type and event that needs to be parsed. These implementation differ slightly between types and events:
- Types and events need to know how to parse themselves. This is encoded into a `Parseable` interface that needs to be implemented by types and events
- Types may appear referenced in the constant pool and thus they need to know how to resolve their own constants. This is encoded into a `Resolvable` interface that needs to be implemented by types.

To add support for new types and events a new data type that satisfies the corresponding interfaces needs to be added, and then included in either the `types` or `events` tables (see [types.go](parser/types.go) and [event_types.go](parser/event_types.go))

## Usage

The parser API is pretty straightforward:

```
func Parse(r io.Reader) ([]Chunk, error)
```

Parser returns a slice of chunks, each containing a slice of events. It should be used like this:

```go
chunks, err := parser.Parse(reader)
```

Check the [main](./main.go) package for further details. It can also be used to validate the parser works with your data and get some basic stats.

## Pending work

The parser is still at an early stage, and you should use it at your own risk (bugs are expected).
The current (non-exhaustive) list of pending work includes:

- Documentation
- Testing
- Annotation support: annotation types are not implemented and annotations are not parsed, they are just ignored.
- Not all string encodings are supported: currently only encodings 0 (null), 1 (empty) and 3 (UTF8 Byte Array) are supported.
- Not all data types are supported. See [types.go](parser/types.go) for a list of supported types.
- Not all event types are supported. See [event_types.go](parser/event_types.go)) for a list of supported event types.

Help with these pending tasks is more than welcome :)

## References

- [JEP 328](https://openjdk.java.net/jeps/328) introduces Java Flight Recorder.
- [async-profiler](https://github.com/jvm-profiling-tools/async-profiler) supports includes a partiar [JFR writer](https://github.com/jvm-profiling-tools/async-profiler/blob/master/src/flightRecorder.cpp) and [reader](https://github.com/jvm-profiling-tools/async-profiler/tree/master/src/converter/one/jfr).
- [JMC](https://github.com/openjdk/jmc) project includes its own [JFR parser](https://github.com/openjdk/jmc/tree/master/core/org.openjdk.jmc.flightrecorder/src/main/java/org/openjdk/jmc/flightrecorder/parser) (in Java).
- [The JDK Flight Recorder File Format](https://www.morling.dev/blog/jdk-flight-recorder-file-format/) by [@gunnarmorling](github.com/gunnarmorling) has a great overview of the JFR format.