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
	// if not present in new dir, send delete request to peer
	for checksum, dir := range oldDir.Dirs {
		targetDir, exists := newDir.Dirs[checksum]
		if exists {
			targetDir.walked = true
			err := compare(oldRootPath, newRootPath, dir, targetDir, syncer, peer)
			if err != nil {
				return err
			}
		} else {
			unrootPath := strings.Replace(dir.Path, oldRootPath, "", 1)
			err := syncer.DeleteDir(oldRootPath, unrootPath, dir, peer)
			if err != nil {
				return err
			}
		}
	}
	// for all directories in new dir that has not been walked,
	// send add request to peer
	for _, dir := range newDir.Dirs {
		if dir.walked {
			continue
		}
		unrootPath := strings.Replace(dir.Path, newRootPath, "", 1)
		err := syncer.AddDir(newRootPath, unrootPath, dir, peer)
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
			unrootPath := strings.Replace(file.Path, oldRootPath, "", 1)
			err := syncer.DeleteFile(oldRootPath, unrootPath, file, peer)
			if err != nil {
				return err
			}
		}
	}
	for _, file := range newDir.Files {
		if file.walked {
			continue
		}
		unrootPath := strings.Replace(file.Path, newRootPath, "", 1)
		err := syncer.AddFile(newRootPath, unrootPath, file, peer)
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
func WalkSubDir(rootPath string, dir *Dir, peer *Peer, fm FileManipulator, dm DirManipulator) error {
	for _, subDir := range dir.Dirs {
		unrootPath := strings.Replace(subDir.Path, rootPath, "", 1)
		if err := dm(rootPath, unrootPath, subDir, peer); err != nil {
			return err
		}
	}
	for _, file := range dir.Files {
		unrootPath := strings.Replace(file.Path, rootPath, "", 1)
		if err := fm(rootPath, unrootPath, file, peer); err != nil {
			return err
		}
	}
	return nil
}
