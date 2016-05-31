package syncbox

import (
	"crypto/md5"
	"flag"
	"os"
	"path/filepath"
)

// Cmd command line options for client program
type Cmd struct {
	RootDir  string
	Username string
	Password string
}

// ParseCommand parse commands for client program
func ParseCommand() (*Cmd, error) {
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		return nil, err
	}
	dir += TestDir
	rootDirPtr := flag.String("root_dir", dir, "the root directory to watch")
	usernamePtr := flag.String("Username", "hello", "Username to login")
	passwordPtr := flag.String("Password", "world", "password to login")
	flag.Parse()
	var pwdSlice []byte
	copy(pwdSlice, *passwordPtr)
	hash := md5.Sum(pwdSlice)
	pwd := string(hash[:])
	return &Cmd{
		RootDir:  *rootDirPtr,
		Username: *usernamePtr,
		Password: pwd,
	}, nil
}

func (c *Cmd) String() string {
	return ToString(c)
}
