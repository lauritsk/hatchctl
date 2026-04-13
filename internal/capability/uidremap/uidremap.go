package uidremap

import (
	"fmt"

	"github.com/lauritsk/hatchctl/internal/backend"
	"github.com/lauritsk/hatchctl/internal/devcontainer"
)

const UpdateScript = `set -eu
REMOTE_USER=$1
NEW_UID=$2
NEW_GID=$3

OLD_UID=
OLD_GID=
HOME_FOLDER=
while IFS=: read -r name _ uid gid _ home _; do
  if [ "$name" = "$REMOTE_USER" ]; then
    OLD_UID=$uid
    OLD_GID=$gid
    HOME_FOLDER=$home
    break
  fi
done < /etc/passwd

EXISTING_USER=
while IFS=: read -r name _ uid _; do
  if [ "$uid" = "$NEW_UID" ]; then
    EXISTING_USER=$name
    break
  fi
done < /etc/passwd

EXISTING_GROUP=
while IFS=: read -r name _ gid _; do
  if [ "$gid" = "$NEW_GID" ]; then
    EXISTING_GROUP=$name
    break
  fi
done < /etc/group

if [ -z "$OLD_UID" ]; then
  echo "Remote user not found in /etc/passwd ($REMOTE_USER)."
elif [ "$OLD_UID" = "$NEW_UID" ] && [ "$OLD_GID" = "$NEW_GID" ]; then
  echo "UIDs and GIDs are the same ($NEW_UID:$NEW_GID)."
elif [ "$OLD_UID" != "$NEW_UID" ] && [ -n "$EXISTING_USER" ]; then
  echo "User with UID exists ($EXISTING_USER=$NEW_UID)."
else
  if [ "$OLD_GID" != "$NEW_GID" ] && [ -n "$EXISTING_GROUP" ]; then
    echo "Group with GID exists ($EXISTING_GROUP=$NEW_GID)."
    NEW_GID=$OLD_GID
  fi
  echo "Updating UID:GID from $OLD_UID:$OLD_GID to $NEW_UID:$NEW_GID."
  sed -i -e "s/\(${REMOTE_USER}:[^:]*:\)[^:]*:[^:]*/\1${NEW_UID}:${NEW_GID}/" /etc/passwd
  if [ "$OLD_GID" != "$NEW_GID" ]; then
    sed -i -e "s/\([^:]*:[^:]*:\)${OLD_GID}:/\1${NEW_GID}:/" /etc/group
  fi
  chown -R "$NEW_UID:$NEW_GID" "$HOME_FOLDER"
fi`

func Desired(resolved devcontainer.ResolvedConfig) bool {
	return resolved.Merged.UpdateRemoteUserUID == nil || *resolved.Merged.UpdateRemoteUserUID
}

func Eligible(resolved devcontainer.ResolvedConfig, image backend.ImageInspect) (string, bool) {
	if !Desired(resolved) {
		return "", false
	}
	imageUser := image.Config.User
	if imageUser == "" {
		imageUser = "root"
	}
	remoteUser := firstNonEmpty(resolved.Merged.RemoteUser, resolved.Merged.ContainerUser, imageUser)
	if remoteUser == "" || remoteUser == "root" || isNumericUser(remoteUser) {
		return "", false
	}
	return remoteUser, true
}

func ExecArgs(containerID string, remoteUser string, uid int, gid int) []string {
	return []string{"exec", "-i", "-u", "root", containerID, "sh", "-s", "--", remoteUser, fmt.Sprintf("%d", uid), fmt.Sprintf("%d", gid)}
}

func isNumericUser(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
