package bridge

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
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
	return copyStreams(conn, stdin, stdout)
}

func helperOpen(args []string) error {
	fs := flag.NewFlagSet("open", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
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
	if *host == "" || *port <= 0 {
		return errors.New("open requires --host and --port")
	}
	conn, err := net.Dial("tcp", net.JoinHostPort(*host, strconv.Itoa(*port)))
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

func copyStreams(target net.Conn, stdin io.Reader, stdout io.Writer) error {
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
	var firstErr error
	for range 2 {
		if err := <-errCh; err != nil && !errors.Is(err, net.ErrClosed) && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
