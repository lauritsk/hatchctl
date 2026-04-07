package bridge

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
)

func HelperMain(args []string) error {
	if len(args) == 0 {
		return errors.New("bridge helper requires a subcommand")
	}
	switch args[0] {
	case "connect":
		return helperConnect(args[1:], os.Stdin, os.Stdout)
	case "open":
		return helperOpen(args[1:])
	case "serve":
		return helperServe(args[1:])
	default:
		return fmt.Errorf("unknown bridge helper subcommand %q", args[0])
	}
}

func helperConnect(args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("connect", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	port := fs.Int("port", 0, "localhost port to connect to")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *port <= 0 {
		return errors.New("connect requires --port")
	}
	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(*port)))
	if err != nil {
		return err
	}
	defer conn.Close()
	copyStreams(conn, stdin, stdout)
	return nil
}

func helperOpen(args []string) error {
	fs := flag.NewFlagSet("open", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	socket := fs.String("socket", filepath.ToSlash(filepath.Join(containerBridgeMountPath, hostSocketName)), "host bridge socket")
	url := fs.String("url", "", "URL to open on the host")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *url == "" {
		return errors.New("open requires --url")
	}
	conn, err := net.Dial("unix", *socket)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := writeBridgeRequest(conn, bridgeRequest{Kind: "open", URL: *url}); err != nil {
		return err
	}
	response, err := readBridgeResponse(conn)
	if err != nil {
		return err
	}
	if !response.OK {
		return errors.New(response.Error)
	}
	return nil
}

func helperServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	socket := fs.String("socket", filepath.ToSlash(filepath.Join(containerBridgeMountPath, helperSocketName)), "container helper socket")
	if err := fs.Parse(args); err != nil {
		return err
	}
	listener, err := listenUnixSocket(*socket)
	if err != nil {
		return err
	}
	defer func() {
		_ = listener.Close()
		_ = os.Remove(*socket)
	}()
	for {
		conn, err := listener.Accept()
		if err != nil {
			if isClosedListener(err) {
				return nil
			}
			return err
		}
		go handleHelperConn(conn)
	}
}

func handleHelperConn(conn net.Conn) {
	defer conn.Close()
	var request bridgeRequest
	if err := json.NewDecoder(conn).Decode(&request); err != nil {
		_ = writeBridgeResponse(conn, bridgeResponse{Error: "invalid request"})
		return
	}
	switch request.Kind {
	case "ping":
		_ = writeBridgeResponse(conn, bridgeResponse{OK: true})
	case "connect":
		if request.Port <= 0 {
			_ = writeBridgeResponse(conn, bridgeResponse{Error: "connect requires port"})
			return
		}
		target, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(request.Port)))
		if err != nil {
			_ = writeBridgeResponse(conn, bridgeResponse{Error: err.Error()})
			return
		}
		defer target.Close()
		if err := writeBridgeResponse(conn, bridgeResponse{OK: true}); err != nil {
			return
		}
		copyStreams(target, conn, conn)
	default:
		_ = writeBridgeResponse(conn, bridgeResponse{Error: "unknown request"})
	}
}

func copyStreams(target net.Conn, stdin io.Reader, stdout io.Writer) {
	errCh := make(chan error, 2)
	go func() {
		_, err := io.Copy(target, stdin)
		closeWrite(target)
		errCh <- err
	}()
	go func() {
		_, err := io.Copy(stdout, target)
		errCh <- err
	}()
	for range 2 {
		if err := <-errCh; err != nil && !errors.Is(err, net.ErrClosed) {
			return
		}
	}
}
