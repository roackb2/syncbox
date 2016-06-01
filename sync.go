package syncbox

// Constants for the action to sync
const (
	ActionAdd    = "add"
	ActionDelete = "delete"
	ActionRename = "rename"
	ActionUpdate = "update"
	ActionGet    = "get"
)

// FileManipulator function type that do CRUD on files
type FileManipulator func(rootPath string, unrootPath string, file *File, peer *Peer) error

// DirManipulator function type that do CRUD on dirs
type DirManipulator func(rootPath string, unrootPath string, dir *Dir, peer *Peer) error

// Syncer is the interface to send file CRUD requests
type Syncer interface {
	AddFile(rootPath string, unrootPath string, file *File, peer *Peer) error
	DeleteFile(rootPath string, unrootPath string, file *File, peer *Peer) error
	AddDir(rootPath string, unrootPath string, dir *Dir, peer *Peer) error
	DeleteDir(rootPath string, unrootPath string, dir *Dir, peer *Peer) error
}
