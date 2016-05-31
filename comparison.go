package syncbox

import "strings"

// Compare should compare to directories and let syncer to deal with the file tree difference.
// This function assumes values of walked of all nodes in dirs are false.
// The caller should give empty string to the path variable
func Compare(oldDir *Dir, newDir *Dir, syncer Syncer, peer *Peer) error {
	err := compare(oldDir.Path, newDir.Path, oldDir, newDir, syncer, peer)
	oldDir.ResetWalked()
	newDir.ResetWalked()
	return err
}

func compare(oldRootPath string, newRootPath string, oldDir *Dir, newDir *Dir, syncer Syncer, peer *Peer) error {
	if oldDir.ContentChecksum == newDir.ContentChecksum {
		return nil
	}
	// for all directories in the old dir, if also exists in new dir, compare them,
	// if not present in new dir, send delete request to server
	for checksum, dir := range oldDir.Dirs {
		targetDir, exists := newDir.Dirs[checksum]
		if exists {
			targetDir.walked = true
			err := compare(oldRootPath, newRootPath, dir, targetDir, syncer, peer)
			if err != nil {
				return err
			}
		} else {
			err := syncer.DeleteDir(strings.Replace(dir.Path, oldRootPath, "", 1), dir, peer)
			if err != nil {
				return err
			}
		}
	}
	// for all directories in new dir that has not been walked,
	// send add request to server
	for _, dir := range newDir.Dirs {
		if dir.walked {
			continue
		}
		err := syncer.AddDir(strings.Replace(dir.Path, newRootPath, "", 1), dir, peer)
		if err != nil {
			return err
		}
	}
	for checksum, file := range oldDir.Files {
		targetFile, exists := newDir.Files[checksum]
		if exists {
			targetFile.walked = true
			continue
		} else {
			err := syncer.DeleteFile(strings.Replace(file.Path, oldRootPath, "", 1), file, peer)
			if err != nil {
				return err
			}
		}
	}
	for _, file := range newDir.Files {
		if file.walked {
			continue
		}
		err := syncer.AddFile(strings.Replace(file.Path, newRootPath, "", 1), file, peer)
		if err != nil {
			return err
		}
	}
	return nil
}

// ResetWalked restes the walked boolean flag to false on all nodes of the file tree
func (dir *Dir) ResetWalked() {
	dir.walked = false
	for _, child := range dir.Dirs {
		child.ResetWalked()
	}
	for _, file := range dir.Files {
		file.walked = false
	}
}

// WalkSubDir walks recursively through sub folders and files and do operations on the files
func WalkSubDir(path string, dir *Dir, peer *Peer, manipulator FileManipulator) error {
	for _, dir := range dir.Dirs {
		err := WalkSubDir(path, dir, peer, manipulator)
		if err != nil {
			return err
		}
	}
	for _, file := range dir.Files {
		err := manipulator(path, file, peer)
		if err != nil {
			return err
		}
	}
	return nil
}
