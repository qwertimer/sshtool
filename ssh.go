package ssh

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type sshconf struct {
	host string
	user string
	pass string
	conn *ssh.Client
	sess *ssh.Session
}

func (sc *sshconf) Init(host, user, pass string) {
	sc.host = host
	sc.user = user
	sc.pass = pass
}

func (sc sshconf) Connect() *ssh.Client {
	var hostkeyCallback ssh.HostKeyCallback
	hostkeyCallback, err := knownhosts.New("~/.ssh/known_hosts")
	if err != nil {
		fmt.Println(err.Error())
	}
	conf := &ssh.ClientConfig{
		User:            sc.user,
		HostKeyCallback: hostkeyCallback,
		Auth: []ssh.AuthMethod{
			ssh.Password(sc.pass),
		},
	}
	sc.conn, err = ssh.Dial("tcp", sc.host, conf)
	if err != nil {
		fmt.Println(err.Error())
	}
	return sc.conn
}

func (sc *sshconf) Session() *ssh.Session {
	sess, err := sc.conn.NewSession()
	sc.sess = sess
	if err != nil {
		fmt.Println(err.Error())
	}
	return sc.sess
}

func (sc sshconf) StartSession(host, user, pass string) *ssh.Session {
	sc.Init(host, user, pass)
	sc.Connect()
	sess := sc.Session()
	return sess
}

func (sc *sshconf) SendCommands(commands ...string) ([]byte, error) {
	modes := ssh.TerminalModes{
		ssh.ECHO:          0,     // disable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}
	session := sc.sess
	err := session.RequestPty("xterm", 80, 40, modes)
	if err != nil {
		return []byte{}, err
	}

	in, err := session.StdinPipe()
	if err != nil {
		log.Fatal("Failed to pipe stdin", err)
	}

	out, err := session.StdoutPipe()
	if err != nil {
		log.Fatal("Failed to pipe stdout", err)
	}

	var output []byte

	go func(in io.WriteCloser, out io.Reader, output *[]byte) {
		var (
			line string
			r    = bufio.NewReader(out)
		)
		for {
			b, err := r.ReadByte()
			if err != nil {
				break
			}

			*output = append(*output, b)

			if b == byte('\n') {
				line = ""
				continue
			}

			line += string(b)
			// sends sudo password if required
		}
	}(in, out, &output)
	cmd := strings.Join(commands, "; ")
	_, err = session.Output(cmd)
	if err != nil {
		return []byte{}, err
	}

	sc.sess.Close()
	return output, nil
}

func (sc *sshconf) StreamCommand(command string) error {
	modes := ssh.TerminalModes{
		ssh.ECHO:          0,     // disable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}
	defer sc.sess.Close()
	err := sc.sess.RequestPty("xterm", 80, 40, modes)
	if err != nil {
		return err
	}

	in, err := sc.sess.StdinPipe()
	if err != nil {
		log.Fatal("Failed to pipe stdin", err)
	}

	out, err := sc.sess.StdoutPipe()
	if err != nil {
		log.Fatal("Failed to pipe stdout", err)
	}
	var wg sync.WaitGroup
	defer wg.Done()
	wg.Add(1)
	quit := make(chan bool)
	go func() {
		for {
			select {
			case <-quit:
				fmt.Println("exiting goroutine")
				return
			default:
				io.Copy(os.Stdout, out)
				io.Copy(in, os.Stdin)
			}
		}
	}()

	if err := sc.sess.Run(command); err != nil {
		log.Fatal("Failed to run: " + err.Error())
	}
	wg.Wait()
	sc.sess.Close()
	return nil
}
