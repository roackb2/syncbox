package syncbox

import "strconv"

// RefGraph is the graph representation structure of file tree nodes and files,
// it should handles CRUD to refs in database
type RefGraph struct {
	*Logger
	Usernmae       string
	User           *UserTable
	FileRecords    []*FileTable
	FileRefRecords []*FileRefTable
	DB             *DB
}

// NewRefGraph instantiates a RefGraph
func NewRefGraph(username string, db *DB) (*RefGraph, error) {
	rg := &RefGraph{
		Usernmae: username,
		DB:       db,
		Logger:   NewLogger(DefaultAppPrefix, GlobalLogInfo, GlobalLogError, GlobalLogDebug),
	}
	userQuery := NewQuery(db)
	userTableSlice := &[]*UserTable{}
	if err := userQuery.Select("*").From("user").Where("username='" + username + "'").Populate(userTableSlice); err != nil {
		return nil, err
	}
	rg.User = (*userTableSlice)[0]

	fileQuery := NewQuery(db)
	if err := fileQuery.Select("*").From("file").Where("user_id='" + strconv.Itoa(rg.User.ID) + "'").Populate(&rg.FileRecords); err != nil {
		return nil, err
	}

	fileRefQuery := NewQuery(db)
	if err := fileRefQuery.Select("*").From("file_ref").Where("user_id='" + strconv.Itoa(rg.User.ID) + "'").Populate(&rg.FileRefRecords); err != nil {
		return nil, err
	}
	// rg.LogDebug("RefGraph: %v\n", rg)
	return rg, nil
}

func (rg *RefGraph) GetRefCount() (int, error) {
	return len(rg.FileRefRecords), nil
}
func (rg *RefGraph) AddRef(file *File, device string, path string) error {
	return nil
}

func (rg *RefGraph) DeleteRef(file *File, device string, path string) error {
	return nil
}
