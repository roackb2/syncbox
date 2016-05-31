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
func NewRefGraph(username string, password string, db *DB) (*RefGraph, error) {
	rg := &RefGraph{
		Usernmae: username,
		DB:       db,
		Logger:   NewDefaultLogger(),
	}
	userQuery := NewQuery(db)
	userTableSlice := &[]*UserTable{}
	if err := userQuery.Select("*").From("user").Where("username='" + username + "'").Populate(userTableSlice); err != nil {
		return nil, err
	}
	if len(*(userTableSlice)) == 0 {
		_, err := db.Exec("INSERT INTO user (username, password) VALUES (?, ?)", username, password)
		if err != nil {
			return nil, err
		}
		if err := userQuery.Select("*").From("user").Where("username='" + username + "'").Populate(userTableSlice); err != nil {
			return nil, err
		}
	}
	rg.User = (*userTableSlice)[0]

	if err := rg.UpdateRecords(); err != nil {
		return nil, err
	}

	return rg, nil
}

// UpdateRecords updates the file and file_ref table records
func (rg *RefGraph) UpdateRecords() error {
	if err := rg.GetFileRecords(); err != nil {
		return err
	}

	if err := rg.GetFileRefRecords(); err != nil {
		return err
	}
	return nil
}

// GetFileRecords gets all file records in database
func (rg *RefGraph) GetFileRecords() error {
	fileQuery := NewQuery(rg.DB)
	if err := fileQuery.Select("*").From("file").Where("user_id='" + strconv.Itoa(rg.User.ID) + "'").Populate(&rg.FileRecords); err != nil {
		return err
	}
	return nil
}

// GetFileRefRecords gets all file_ref records in database
func (rg *RefGraph) GetFileRefRecords() error {
	fileRefQuery := NewQuery(rg.DB)
	if err := fileRefQuery.Select("*").From("file_ref").Where("user_id='" + strconv.Itoa(rg.User.ID) + "'").Populate(&rg.FileRefRecords); err != nil {
		return err
	}
	return nil
}

// GetRefCount returns the file_ref count
func (rg *RefGraph) GetRefCount() (int, error) {
	return len(rg.FileRefRecords), nil
}

// AddFileRecord add a record to file table
func (rg *RefGraph) AddFileRecord(file *File, path string, device string) error {
	_, err := rg.DB.Exec("INSERT INTO file (checksum, user_id) VALUES (?, ?)", ChecksumToNumString(file.ContentChecksum), rg.User.ID)
	if err != nil {
		return err
	}

	if err := rg.UpdateRecords(); err != nil {
		return err
	}

	return nil
}

// AddFileRefRecord add a record to file_ref table
func (rg *RefGraph) AddFileRefRecord(file *File, path string, device string) error {
	query := NewQuery(rg.DB)
	fileRecords := &[]*FileTable{}
	err := query.Select("*").From("file").Where("checksum='" + ChecksumToNumString(file.ContentChecksum) + "'").Populate(fileRecords)
	if err != nil {
		return err
	}
	fileRecord := (*fileRecords)[0]

	_, err = rg.DB.Exec("INSERT INTO file_ref (user_id, file_id, path, device) VALUES (?, ?, ?, ?)", rg.User.ID, fileRecord.ID, path, device)
	if err != nil {
		return err
	}

	if err := rg.UpdateRecords(); err != nil {
		return err
	}

	return nil
}

// DeleteFileRecord deletes a record in file table
func (rg *RefGraph) DeleteFileRecord(file *File, device string, path string) error {
	query := NewQuery(rg.DB)
	fileRecords := &[]*FileTable{}
	err := query.Select("*").From("file").Where("checksum='" + ChecksumToNumString(file.ContentChecksum) + "'").Populate(fileRecords)
	if err != nil {
		return err
	}
	fileRecord := (*fileRecords)[0]

	_, err = rg.DB.Exec("DELETE FROM file WHERE id=?", fileRecord.ID)
	if err != nil {
		return err
	}

	if err := rg.UpdateRecords(); err != nil {
		return err
	}

	return nil
}

// DeleteFileRefRecord deletes a record in file_ref table
func (rg *RefGraph) DeleteFileRefRecord(file *File, device string, path string) error {
	query := NewQuery(rg.DB)
	fileRecords := &[]*FileTable{}
	err := query.Select("*").From("file").Where("checksum='" + ChecksumToNumString(file.ContentChecksum) + "'").Populate(fileRecords)
	if err != nil {
		return err
	}
	if len(*fileRecords) == 0 {
		return ErrorNoFileRecords
	}
	fileRecord := (*fileRecords)[0]

	_, err = rg.DB.Exec("DELETE FROM file_ref WHERE file_id=?", fileRecord.ID)
	if err != nil {
		return err
	}

	if err := rg.UpdateRecords(); err != nil {
		return err
	}

	return nil
}

// GetNoRefFiles returns file records that has no references on them
func (rg *RefGraph) GetNoRefFiles() ([]*FileTable, error) {
	noRefFiles := make([]*FileTable, 0, 0)
	for _, fileRecord := range rg.FileRecords {
		count := 0
		err := rg.DB.QueryRow("SELECT COUNT(*) as cnt FROM file_ref WHERE file_id=?;", fileRecord.ID).Scan(&count)
		if err != nil {
			return nil, err
		}
		if count == 0 {
			noRefFiles = append(noRefFiles, fileRecord)
		}
	}
	return noRefFiles, nil
}
