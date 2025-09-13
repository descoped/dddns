package constants

import "os"

// File and directory permissions used throughout dddns
const (
	// ConfigFilePerm is the standard config file permission (owner read/write only)
	ConfigFilePerm os.FileMode = 0600 // rw-------

	// SecureConfigPerm is the permission for encrypted config (owner read only)
	SecureConfigPerm os.FileMode = 0400 // r--------

	// ConfigDirPerm is the permission for config directories (owner full access)
	ConfigDirPerm os.FileMode = 0700 // rwx------

	// CacheDirPerm is the permission for cache directories
	CacheDirPerm os.FileMode = 0755 // rwxr-xr-x

	// CacheFilePerm is the permission for cache files
	CacheFilePerm os.FileMode = 0600 // rw-------
)