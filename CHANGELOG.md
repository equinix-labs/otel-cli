## [0.4.3] - 2024-03-11

Add injection of `{{traceparent}}` to `otel-cli exec` as default behavior, along with
the `otel-cli exec --tp-disable-inject` to turn it off (old behavior).

### Added

- `otel-cli exec echo {{traceparent}}` is now supported to pass traceparent to child process
- `otel-cli exec --tp-disable-inject` will disable this new default behavior

## [0.4.2] - 2023-12-01

The Docker container now builds off `alpine:latest` instead of `scratch`. This
makes the default certificate store included with Alpine available to otel-cli.

### Changed

- switch release Dockerfile to base off alpine:latest

## [0.4.1] - 2023-10-16

Mostly small but impactful changes to `otel-cli exec`.

### Added

- `otel-cli exec --command-timeout 30s` provides a separate command timeout from the otel timeout
- SIGINT is now caught and passed to the child process
- attributes can be set or overwrite on a backgrounded span via `otel-cli span end`

### Changed

- bumped several dependencies to the latest release
- updated README.md

## [0.4.0] - 2023-08-09

This focus of this release is a brand-new OTLP client implementation. It has fewer features
than the opentelemetry-collector code, and allows for more fine-grained control over how
gRPC and HTTP are configured. Along the way, the `otelcli` and `otlpclient` packages went
through a couple refactors to organize code better in preparation for adding metrics and
logs, hopefully in 0.5.0.

### Added

- `--force-parent-span-id` allows forcing the span parent (thanks @domofactor!)
- `otel-cli status` now includes a list of errors including retries that later succeeded

### Changed

- `--otlp-blocking` is marked deprecated and no longer does anything
- the OTLP client implementation is no longer using opentelemetry-collector
- traceparent code is now in a w3c/traceparent package
- otlpserver.CliEvent is removed entirely, preferring protobuf spans & events

## [0.3.0] - 2023-05-26

The most important change is that `otel-cli exec` now treats arguments as an argv
list instead of munging them into a string. This shouldn't break anyone doing sensible
things, but could if folks were expecting the old `sh -c` behavior.

Envvars are no longer deleted before calling exec. Actually, they still are, but otel-cli
backs up its envvars early so they can be propagated anyways.

The rest of the visible changes are incremental additions or fixes. As for the invisible,
otel-cli now generates span protobufs directly, and no longer goes through the
opentelemetry-go SDK. This ended up making some code cleaner, aside from some of the
protobuf-isms that show up.

A user has a use case for setting custom trace and span ids, so the `--force-trace-id`
and `--force-span-id` options were added to `span`, `exec`, and other span-generating
subcommands.

### Added

- `--force-trace-id` and `--force-span-id`
- `--status-code` and `--status-description` are supported, including on `otel-cli span background end`
- demo for working around race conditions when using `span background` 
- more functional tests, including some regression tests
- functional test harness now has a way to do custom checks via CheckFuncs
- added this changelog

### Changed

- (behavior) reverted envvar deletion so envvars will propagate through `exec` again
- (behavior) exec argument handling is now precise and no longer uses `sh -c`
- build now requires Go 1.20 or greater
- otel-cli now generates span protobufs directly instead of using opentelemetry-go
- respects signal-specific configs per OTel spec
- handle endpoint configs closer to spec
- lots of little cleanups across the code and docs
- many dependency updates

## [0.2.0] 2023-02-27

The main addition in this version is client mTLS authentication support, which comes in with
extensive e2e tests for TLS settings.

`--no-tls-verify` is deprecated in favor of `--tls-no-verify` so all the TLS options are consistent.

`otel-cli span background` now has a `--background-skip-parent-pid-check` option for some use cases
where folks want otel-cli to keep running past its parent process.

### Changed

- 52f1143 #11 support OTEL_SERVICE_NAME per spec (#158)
- ed4bf2f Bump golang.org/x/net from 0.5.0 to 0.7.0 (#159)
- 5c5865c Make configurable skipping pid check in span background command (#161)
- 7214b64 Replace Jaeger instructions with otel-desktop-viewer (#162)
- 6018f76 TLS naming cleanup (#166)
- 9a7de86 add TLS testing and client certificate auth (#150)
- 759fbef miscellaneous documentation fixes (#165)
- f5286c0 never allow insecure automatically for https:// URIs (#149)

## [0.1.0] 2023-02-02

Apologies for the very long delay between releases. There is a lot of pent-up change
in this release.

Bumped minor version to 0.1 because there are some changes in behavior around
endpoint URI handling and configuration. Also some inconsistencies in command line
arguments has been touched up, so some uses of single-letter flags and `--ignore-tp-env`
(renamed to `--tp-ignore-env to match other flags) might break.

Viper has been dropped in favor of directly loading configuration from json and
environment variables. It appears none of the Viper features ever worked in
otel-cli so it shouldn't be a big deal, but if you were using Viper configs they
won't work anymore and you'll have to switch to otel-cli's json config format.

Endpoints now conform mostly to the OTel spec, except for a couple cases
documented in the README.md.

### Changed

- 4256644 #108 fix span background attrs (#116)
- e8b86f6 #142 follow spec for OTLP protocol (#148)
- efb5608 #42 add version subcommand (#114)
- 007f8f7 Add renovate.json (#123)
- 9d7a668 Make the service/name CLI short args match the long args (#110)
- b164427 add http testing (#143)
- 72df644 docs: --ignore-tp-env replace field to --tp-ignore-env (#147)
- e48e468 feat: add span status code/description cli and env var (#111)
- ce850f4 make grpc server stop more robust (#122)
- 8eb37fb remove viper, fix tests, fix and expand envvars (#120)
- ff5a4eb update OTel to 1.4.1 (#107)
- b51d6fc update goreleaser config, add release procedure in README.md (#141)
- 99c9242 update opentelemetry SDK to 1.11.2 (#138)

## [0.0.x] - 2021-03-24 - 2022-02-24

Developing the base functionality. Light on testing.

### Changed

- added OTLP test server
- added goreleaser
- added timeouts
- many refactors while discovering the shape of the tool
- switch to failing silently
- added subcommand to generate autocomplete data
- added status subcommand
- added functional test harness
- added HTTP support
