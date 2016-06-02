package syncbox

import (
	"crypto/md5"
	"flag"
	"os"
	"path"
	"path/filepath"
)

// Cmd command line options for client program
type Cmd struct {
	RootDir  string
	TmpDir   string
	Username string
	Password string
}

// ParseCommand parse commands for client program
func ParseCommand() (*Cmd, error) {
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		return nil, err
	}

	tempDir := path.Join(getSystemTempDir(), "syncbox")
	dir += TestDir
	rootDirPtr := flag.String("root_dir", dir, "the root directory to watch")
	tmpDirPtr := flag.String("tmp_dir", tempDir, "the temporary folder to put files that deleted")
	usernamePtr := flag.String("Username", "hello", "username to login")
	passwordPtr := flag.String("Password", "world", "password to login")
	flag.Parse()
	var pwdSlice []byte
	copy(pwdSlice, *passwordPtr)
	hash := md5.Sum(pwdSlice)
	pwd := string(hash[:])
	return &Cmd{
		RootDir:  *rootDirPtr,
		TmpDir:   *tmpDirPtr,
		Username: *usernamePtr,
		Password: pwd,
	}, nil
}

func (c *Cmd) String() string {
	return ToString(c)
}

func getSystemTempDir() string {
	tmp := os.Getenv("TMPDIR")
	if tmp == "" {
		tmp = "/tmp"
	}
	return tmp
}
