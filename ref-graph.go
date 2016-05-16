package syncbox

// RefGraph is the graph representation structure of file tree nodes and files,
// it should handles CRUD to refs in database
type RefGraph struct {
	Usernmae       string
	User           *UserTable
	FileRecords    []*FileTable
	FileRefRecords []*FileRefTable
}

func NewRefGraph(username string) (*RefGraph, error) {
	return nil, nil
}

func (rg *RefGraph) GetRefCount() (int, error) {
	return -1, nil
}
func (rg *RefGraph) AddRef(file *File, device string, path string) error {
	return nil
}

func (rg *RefGraph) DeleteRef(file *File, device string, path string) error {
	return nil
}
