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

	errCh := make(chan error, 2)
	go func() {
		_, err := io.Copy(conn, stdin)
		if tcp, ok := conn.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
		errCh <- err
	}()
	go func() {
		_, err := io.Copy(stdout, conn)
		errCh <- err
	}()
	for range 2 {
		if err := <-errCh; err != nil && !errors.Is(err, net.ErrClosed) {
			return err
		}
	}
	return nil
}
