# springboard

_a server and client implementing Spring '83_

This server and client aim to track the latest drafts of the spring89 protocol specification.
Currently this implementation is targetting [draft-20220616](https://github.com/robinsloan/spring-83-spec/blob/main/draft-20220616.md).

You can see it in action at [spring83.kindrobot.ca](https://spring83.kindrobot.ca). (Come say hi!)

## Post a board

You can download precompiled binaries for your system / architecture from the releases tab.

Run `./springboard help` (or `.\springboard.exe help` on Windows) to get started posting to a spring83 server.

An example:

```bash
./springboard generate-keys
echo "<p>Hello, world!</p>" > board.html
./springboard post https://spring83.kindrobot.ca < board.html
```

`./springboard generate-keys` may take several minutes and use a lot of proccessing power.
By default, it will save the key pair to `$HOME/.config/spring83`. 

`board.html` can be any valid-ish HTML5 document. It may not have scripts or
load externl resources. You should not put:

```html
<time datatime="...">
```

in `board.html`, springboard will do this for you.

### Help, my key doesn't work

The key format changed between draft versions. You may need to upgrade your client to v1 or greater, delete the contents of `~/.config/spring83`, and try again.

## Run a server

You can run a server with:

```bash
springboard serve PATH_TO_CONFIG_YAML
```

Where a the schema of the file at `PATH_TO_CONFIG_YAML` is:

```yaml
---
# port on which to serve this server
port: 8000
# boards to which to propagate new boards
federates:
  - https://spring83.kindrobot.ca
  - https://0l0.lol
  - https://bogbody.biz
  - https://spring83.mozz.us/
# who the server will say it is during propagation
fqdn: localhost:8000
# how long to wait until propagating a new board
propagate_wait: 5m
# the board to "pin" at the top
admin_board: bf71bb0d73bc3b0edfd0bd750f9e191c476773b3660d9ba86d658b49083e0623
```

Alternatively you can specify the following environment variables respectively:

* `PORT`
* `SB_FEDERATES`
* `SB_FQDN`
* `SB_PROPAGATE_WAIT`
* `SB_ADMIN_BOARD`

## Hacking

### run the server

Run `PORT=8000 go run cmd/springboard serve`.  `PORT` is optional and defaults to 8000.

### run the client

```bash
go run ./cmd/springboard generate-keys # will take a while, only need to run once
echo "<p>Hello, world!</p>" | go run ./cmd/springboard post https://localhost:8000
```

### view the content

go to http://localhost:8000 while the server is running

## Other known Spring '83 implementations
| Name                       | Lang                | Instance                 |
| -------------------------- | ------------------- | -------------------------|
| [llimllib/springer]        | golang              |                          |
| [michael-lazar/lets-dance] | python              | https://spring83.mozz.us |
| [rpj/spring83]             | Javascript (NodeJS) | https://0l0.lol/         |

## Credits

- this package was forked from [llimllib/springer](https://github.com/llimllib/springer/)
- the protocol (and original newsletter) is/was by [Robin Sloan](https://www.robinsloan.com/lab/specifying-spring-83/)

## License

This work is released into the public domain.

[llimllib/springer]: https://github.com/llimllib/springer
[michael-lazar/lets-dance]: https://github.com/michael-lazar/lets-dance
[rpj/spring83]: https://github.com/rpj/spring83
