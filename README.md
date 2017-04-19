rerun: Re-run command on file system changes
======

Lightweight file-watcher that re-runs command on FS changes. It has simple CLI and optional config file. By default, it uses 200ms delay, which gives enough time for tools like git to update all directories/files within repository before killing the old process.

**In development. Only CLI MVP works right now.**

# Usage
## `rerun [-watch DIRS...] [-ignore DIRS...] -run COMMAND [ARG...]`

```bash
rerun -watch $(go list ./...) -ignore vendor bin -run go test -run=YourTest
```

# Installation

```bash
go get -u github.com/VojtechVitek/rerun/cmd/rerun
```
*[Download Go here](https://golang.org/dl/).*

*TODO: Release page.*

# TODO

- [ ] Versioning + (Releases)[/releases]
- [ ] Regexp matches
- [ ] Verbose mode
- [ ] Interactive mode
- [ ] Config file, as an alternative to direct CLI invocation

```yaml
api:
  watch:
    - cmd
    - *.go
  ignore:
    - bin
    - *_test.go
  cmd:
    - go run cmd/api/main.go -flags args

test-login:
  name: Test login
  watch:
    - tests/e2e
    - services/auth
    - data
  run:
    - go test -run=Login
```


Written in [golang](https://github.com/golang/go). Uses [fsnotify](https://github.com/fsnotify/fsnotify) behind the scenes, so technically it should work on most platforms including Linux, Mac OS and Windows.

# License

Licensed under the [MIT License](./LICENSE).
