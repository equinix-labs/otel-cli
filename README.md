## Synopsis

otel-cli is a command-line tool for sending OpenTelemetry events. It is written in
Go and intended to be used in shell scripts and other places where the best option
available for sending events is executing another program.


Note: just made-up examples for now for me to think through the CLI parameters

```
# defaults to stdout
otel-cli -e FOO -k someKey -v someValue

# writes to stdout and to the forwarder
otel-cli --stdout --otlp 

# (maybe bad) idea - persist context to a tempfile across executions
otel_context=$(mktemp)
otel-cli --context $otel_context --stdout ...

# maybe will require a flag library more advanced than stdlib flag but...
# this would be cool for catching command name, performance data, and exit codes automatically
# ala time and maybe even some of the syscalls that get performance data about child processes
# like memory usage & stuff
otel-cli --exec sleep 10
```

## Unknowns

   * should this include Jaeger/Zipkin exporters in every build for simplicity?
      * for now, just stdout & otlp

## Ideas

   * `time(1)` style traces, e.g. `otel-cli -exec "$command"`
   * a --spanContext file path or similar for persisting span context across executions

## License

Proprietary

BUT: I'm writing this so it can be open sourced later and do wish to avoid making anything
Equinix or observability provider specific.
