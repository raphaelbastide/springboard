# A testing Spring83 server

Implementing some of [this very draft spec](https://github.com/robinsloan/spring-83-spec/blob/main/draft-20220609.md)

## Running

You can download precompiled binaries for your system / architecture from the releases tab.

Run `./s93client --help` (or `.\s93client.exe --help` on Windows) to get started posting to a spring93 server.

## Hacking

## run the server

If you have [modd]() installed, run `modd`. Alternatively, `PORT=8000 go run cmd/s93server/main.go`.
`PORT` is optional and defaults to 8000.

## run the client

On first run, the client will generate a keypair for you according to the spring83 spec, and store it in `~/.config/spring83/key.pub` and `~/.config/spring83/key.priv`.

This key has to meet a certain specification, so it may take some time to generate on the first run.

`echo "<em>very</em> <pre>cool</pre>" | go run cmd/s93client/main.go http://localhost:8000`

Run `go run cmd/s93client/main.go --help` for all options.

## view the content

go to http://localhost:8000 while the server is running
