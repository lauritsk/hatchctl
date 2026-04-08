package bridge

import (
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
	socket := fs.String("socket", "", "host bridge socket")
	host := fs.String("host", "", "host bridge address")
	port := fs.Int("port", 0, "host bridge port")
	token := fs.String("token", "", "host bridge token")
	url := fs.String("url", "", "URL to open on the host")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *url == "" {
		return errors.New("open requires --url")
	}
	conn, err := dialHelperOpen(*socket, *host, *port)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := writeBridgeRequest(conn, bridgeRequest{Kind: "open", URL: *url, Token: *token}); err != nil {
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

func dialHelperOpen(socket string, host string, port int) (net.Conn, error) {
	if host != "" && port > 0 {
		return net.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	}
	if socket == "" {
		socket = filepath.ToSlash(filepath.Join(containerBridgeMountPath, hostSocketName))
	}
	return net.Dial("unix", socket)
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
