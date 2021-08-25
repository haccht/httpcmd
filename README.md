# httpcmd
`httpcmd` is a simple command line tool that serves your CLI tools over http.

## Usage

Run `httpcmd` with your preferred command as its arguments.

```bash
$ httpcmd --addr 127.0.0.1:8080 tail -f /var/log/syslog
```

On the client side, you can invoke the command using `curl`.

```bash
$ curl -sN http://127.0.0.1:8080
```

## Options

```
$ httpcmd -h
Usage:
  httpcmd [OPTIONS] <command>

Application Options:
  -a, --addr=            Address to listen on (default: :8080)
  -x, --exec             Pass command to exec instead of "sh -c"
      --permit-argument  Permit clients to send command line arguments in URL (e.g. http://example.com:8080/?arg=AAA&arg=BBB)
  -v, --version          Output version information and exit

Help Options:
  -h, --help             Show this help message
```
