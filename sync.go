package syncbox

const (
	actionAdd    = "add"
	actionDelete = "delete"
	actionRename = "rename"
	actionUpdate = "update"
)

// FileManipulator function type that do CRUD on files
type FileManipulator func(path string, file *File) error

// Syncer is the interface to send file CRUD requests
type Syncer interface {
	AddFile(path string, file *File) error
	DeleteFile(path string, file *File) error
	GetFile(path string, file *File) error
	AddDir(path string, dir *Dir) error
	DeleteDir(path string, dir *Dir) error
}

// FileManipRequest represents operations to do on files
type FileManipRequest struct {
	Action string
	File   *File
}
