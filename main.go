package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/creack/pty"
	flags "github.com/jessevdk/go-flags"
)

const Version = "1.0.3"

type config struct {
	Addr       string `short:"a" long:"addr"    description:"Address to listen on" default:":8080"`
	Exec       bool   `short:"x" long:"exec"    description:"Pass command to exec instead of \"sh -c\""`
	PermitArgs bool   `long:"permit-argument"   description:"Permit clients to send command line arguments in URL (e.g. http://example.com:8080/?arg=AAA&arg=BBB)"`
	Version    func() `short:"v" long:"version" description:"Output version information and exit"`
}

func listen(addr string) (net.Listener, error) {
	if strings.Contains(addr, "/") {
		return net.Listen("unix", addr)
	}

	return net.Listen("tcp", addr)
}

func listenAndServe(addr string, mux *http.ServeMux) error {
	fmt.Printf("Listening on %s\n", addr)

	conn, err := listen(addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
		<-sigCh

		conn.Close()
		os.Exit(1)
	}()

	return http.Serve(conn, mux)
}

func commandFunc(opts config, cmdArgs []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(strings.ToLower(r.Header.Get("User-Agent")), "curl") {
			return
		}

		args := make([]string, len(cmdArgs))
		copy(args, cmdArgs)

		if opts.PermitArgs {
			query := r.URL.Query()
			args = append(args, query["arg"]...)
		}

		var cmd *exec.Cmd
		if opts.Exec {
			cmd = exec.Command(args[0], args[1:]...)
		} else {
			cmd = exec.Command("sh", "-c", strings.Join(args, " "))
		}

		ptmx, err := pty.Start(cmd)
		if err != nil {
			fmt.Fprintln(w, err)
			return
		}
		defer ptmx.Close()

		ch := make(chan []byte)
		go func() {
			defer close(ch)

			buf := make([]byte, 512)
			for {
				n, err := ptmx.Read(buf)
				if err != nil {
					break
				}

				dup := make([]byte, n)
				copy(dup, buf)

				ch <- dup
			}
		}()

		tick := time.NewTicker(1 * time.Second)
		defer tick.Stop()

	L:
		for {
			select {
			case <-r.Context().Done():
				cmd.Process.Kill()
				break L
			case buf, ok := <-ch:
				if ok != true {
					break L
				}
				w.Write(buf)
			case <-tick.C:
				// send NUL to keep the http connection alive
				w.Write([]byte("\x00"))
			}

			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}

		if err := cmd.Wait(); err != nil {
			fmt.Fprintln(w, err)
			return
		}
	}
}

func main() {
	var opts config
	opts.Version = func() {
		fmt.Println(Version)
		os.Exit(1)
	}

	parser := flags.NewParser(&opts, flags.Default|flags.PassAfterNonOption)
	parser.Usage = "[options] <command>"

	cmdArgs, err := parser.Parse()
	if err != nil {
		os.Exit(1)
	} else if len(cmdArgs) < 1 {
		parser.WriteHelp(os.Stderr)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", commandFunc(opts, cmdArgs))

	log.Fatal(listenAndServe(opts.Addr, mux))
}
