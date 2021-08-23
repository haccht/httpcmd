package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	flags "github.com/jessevdk/go-flags"
)

const Version = "1.0.0"

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

func combinedOutputChannel(cmd *exec.Cmd) (chan []byte, error) {
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	r := io.MultiReader(stdoutPipe, stderrPipe)
	ch := make(chan []byte)

	go func() {
		defer close(ch)

		buf := make([]byte, 512)
		for {
			n, err := r.Read(buf)
			if err != nil {
				if err != io.EOF {
					ch <- []byte(err.Error())
				}
				break
			}

			dup := make([]byte, n)
			copy(dup, buf)

			ch <- dup
		}
	}()

	return ch, nil
}

func writeResponse(w io.Writer, ch chan []byte) {
	tick := time.NewTicker(1 * time.Second)
	defer tick.Stop()

	for {
		select {
		case chunk, ok := <-ch:
			if ok != true {
				return
			}
			w.Write(chunk)
		case <-tick.C:
			// send NUL to keep the http connection alive
			w.Write([]byte("\x00"))
		}

		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
}

func commandFunc(opts config, cmdArgs []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(strings.ToLower(r.Header.Get("User-Agent")), "curl") {
			return
		}

		if opts.PermitArgs {
			query := r.URL.Query()
			cmdArgs = append(cmdArgs, query["arg"]...)
		}

		var cmd *exec.Cmd
		if opts.Exec {
			cmd = exec.Command(cmdArgs[0], cmdArgs[1:]...)
		} else {
			cmd = exec.Command("sh", "-c", strings.Join(cmdArgs, " "))
		}

		ch, err := combinedOutputChannel(cmd)
		if err != nil {
			fmt.Fprintln(w, err)
			return
		}

		if err := cmd.Start(); err != nil {
			fmt.Fprintln(w, err)
			return
		}

		writeResponse(w, ch)
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
