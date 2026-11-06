//go:build !windows

package provider

import (
	"os"
	"os/user"
)

func sudoUserHome() (string, bool) {
	if os.Geteuid() != 0 {
		return "", false
	}
	uid := os.Getenv("SUDO_UID")
	if uid == "" || uid == "0" {
		return "", false
	}
	account, err := user.LookupId(uid)
	if err != nil {
		return "", false
	}
	return account.HomeDir, account.HomeDir != ""
}
