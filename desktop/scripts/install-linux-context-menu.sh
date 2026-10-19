#!/usr/bin/env bash
# Register "Send to Beginning Android" in Nautilus / Nemo / Thunar file managers

ACTIONS_DIR="${HOME}/.local/share/file-manager/actions"
mkdir -p "$ACTIONS_DIR"

cat <<EOF > "${ACTIONS_DIR}/beginning-android.desktop"
[Desktop Entry]
Type=Action
Name=Send to Beginning Android
Icon=beginning-android
Profiles=profile-zero;

[X-Action-Profile profile-zero]
MimeTypes=all/allfiles;
Exec=beginning-android --send-file %f
Name=Default
EOF

echo "Linux context menu action installed successfully."
