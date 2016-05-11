package syncbox

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"
)

// constants for files
const (
	indent         = "\t" // Constants about files
	DigestFileName = ".sb-digest.json"
	TestDir        = "/test-target"
)

// Checksum alias [16]byte to Checksum
type Checksum [16]byte

// Files alias of map of files
type Files map[Checksum]*File

// Dirs alias of map of directories
type Dirs map[Checksum]*Dir

// Object base type of file system objects
type Object struct {
	IsDir           bool        `json:"isDir"`
	ModTime         time.Time   `json:"modTime"`
	Mode            os.FileMode `json:"mode"`
	Name            string      `json:"name"`
	Size            int64       `json:"size"`
	ContentChecksum Checksum    `json:"contentChecksum"`
	Path            string      `json:"path"`
	walked          bool
}

// Dir directory representation
type Dir struct {
	*Object
	Files Files `json:"files"`
	Dirs  Dirs  `json:"dirs"`
}

// File file representation
type File struct {
	*Object
}

// NewObject instantiate Object
func NewObject(info os.FileInfo, path string) *Object {
	return &Object{
		IsDir:           info.IsDir(),
		ModTime:         info.ModTime(),
		Mode:            info.Mode(),
		Name:            info.Name(),
		Size:            info.Size(),
		ContentChecksum: Checksum{},
		Path:            path,
		walked:          false,
	}
}

// NewDir instantiate Dir
func NewDir(info os.FileInfo, path string) *Dir {
	return &Dir{
		Object: NewObject(info, path),
		Files:  make(Files),
		Dirs:   make(Dirs),
	}
}

// NewFile instantiate File
func NewFile(info os.FileInfo, path string) *File {
	return &File{
		Object: NewObject(info, path),
	}
}

func (o *Object) String() string {
	return o.toString(0)
}

func (dir *Dir) String() string {
	return dir.toString(0)
}

func (f *File) String() string {
	return f.Object.toString(0)
}

func (o *Object) toString(depth int) string {
	prefix := strings.Repeat(indent, depth)
	str := fmt.Sprintf("%vPath: %v\n", prefix, o.Path)
	str += fmt.Sprintf("%vIsDir: %v\n", prefix, o.IsDir)
	str += fmt.Sprintf("%vModTime: %v\n", prefix, o.ModTime)
	str += fmt.Sprintf("%vMode: %v\n", prefix, o.Mode)
	str += fmt.Sprintf("%vName: %v\n", prefix, o.Name)
	str += fmt.Sprintf("%vSize: %v\n", prefix, o.Size)
	str += fmt.Sprintf("%vContentChecksum: %v\n", prefix, o.ContentChecksum)
	return str
}

func (dir *Dir) toString(depth int) string {
	prefix := strings.Repeat(indent, depth)
	str := fmt.Sprintf("%v", dir.Object.toString(depth))
	str += fmt.Sprintf("%vfiles:\n", prefix)
	for checksum, file := range dir.Files {
		str += fmt.Sprintf("%v%v%v:\n%v\n", indent, prefix, checksum, file.toString(depth+1))
	}
	str += fmt.Sprintf("%vdirs:\n", prefix)
	for checksum, dir := range dir.Dirs {
		str += fmt.Sprintf("%v%v%v:\n%v\n", indent, prefix, checksum, dir.toString(depth+1))
	}
	return str
}

// MarshalJSON implements the json interface
// Files is map, no need to pass by pointer
func (fs Files) MarshalJSON() ([]byte, error) {
	strMap := make(map[string]*File)
	for checksum, file := range fs {
		strMap[string(checksum[:])] = file
	}
	return json.Marshal(strMap)
}

// UnmarshalJSON implements the json interface
// pass by reference because needs to change the target address of map reference
func (fs *Files) UnmarshalJSON(data []byte) error {
	*fs = make(map[Checksum]*File)
	strMap := make(map[string]*File)
	if err := json.Unmarshal(data, &strMap); err != nil {
		return err
	}
	for key, file := range strMap {
		checksum := [16]byte{}
		copy(checksum[:], []byte(key))
		(*fs)[checksum] = file
	}
	return nil
}

// MarshalJSON json interface
// Dirs is map, no need to pass by pointer
func (ds Dirs) MarshalJSON() ([]byte, error) {
	strMap := make(map[string]*Dir)
	for checksum, dir := range ds {
		strMap[string(checksum[:])] = dir
	}
	return json.Marshal(strMap)
}

// UnmarshalJSON json interface
// pass by reference because needs to change the target address of map reference
func (ds *Dirs) UnmarshalJSON(data []byte) error {
	*ds = map[Checksum]*Dir{}
	strMap := make(map[string]*Dir)
	if err := json.Unmarshal(data, &strMap); err != nil {
		return err
	}
	for key, dir := range strMap {
		var checksum Checksum
		copy(checksum[:], []byte(key))
		(*ds)[checksum] = dir
	}
	return nil
}

// ToJSON converts Dir to JSON string
func (dir *Dir) ToJSON() (string, error) {
	jsonBytes, err := json.Marshal(dir)
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

// RebornDir reborn directory from JSON string
func RebornDir(jsonStr string) (*Dir, error) {
	jsonBytes := []byte(jsonStr)
	restoredDir := Dir{}
	if err := json.Unmarshal(jsonBytes, &restoredDir); err != nil {
		return nil, err
	}
	return &restoredDir, nil
}

// Build walks through content of the path directory and builds a tree representation
func Build(path string) (*Dir, Checksum, error) {
	var parentDir *Dir
	var totalChecksum Checksum
	parentDirFile, err := os.Open(path)
	if err != nil {
		return parentDir, totalChecksum, err
	}
	parentDirInfo, err := parentDirFile.Stat()
	if err != nil {
		return parentDir, totalChecksum, err
	}
	parentDir = NewDir(parentDirInfo, path)

	digest := md5.New()
	infos, err := ioutil.ReadDir(path)
	if err != nil {
		return parentDir, totalChecksum, err
	}
	for _, info := range infos {
		if info.IsDir() {
			dir, checksum, err := Build(path + "/" + info.Name())
			if err != nil {
				return parentDir, totalChecksum, err
			}
			parentDir.Dirs[checksum] = dir
			digest.Write(checksum[:])
		} else if info.Name() != DigestFileName {
			content, err := ioutil.ReadFile(path + "/" + info.Name())
			if err != nil {
				return parentDir, totalChecksum, err
			}
			checksum := md5.Sum(content)
			file := NewFile(info, path+"/"+info.Name())
			file.ContentChecksum = checksum
			parentDir.Files[checksum] = file
			digest.Write(checksum[:])
		}
		copy(totalChecksum[:], digest.Sum(nil))
		parentDir.ContentChecksum = totalChecksum
	}
	return parentDir, totalChecksum, nil
}
