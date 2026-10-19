#!/usr/bin/env bash
# Beginning Android - Linux Context Menu & Desktop Installer
set -e

echo "=== Installing Beginning Android for Linux ==="

INSTALL_DIR="${HOME}/.local/bin"
mkdir -p "${INSTALL_DIR}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SOURCE_BIN="${SCRIPT_DIR}/bin/beginning-android-linux-amd64"

if [ -f "${SOURCE_BIN}" ]; then
    cp "${SOURCE_BIN}" "${INSTALL_DIR}/beginning-android"
    chmod +x "${INSTALL_DIR}/beginning-android"
    echo "[OK] Installed binary to ${INSTALL_DIR}/beginning-android"
fi

# 1. Nautilus / Nemo / Caja File Manager Script
NAUTILUS_SCRIPTS="${HOME}/.local/share/nautilus/scripts"
mkdir -p "${NAUTILUS_SCRIPTS}"

cat <<'EOF' > "${NAUTILUS_SCRIPTS}/Send to Beginning Android"
#!/usr/bin/env bash
for file in "$@"; do
    "${HOME}/.local/bin/beginning-android" send "$file"
done
EOF
chmod +x "${NAUTILUS_SCRIPTS}/Send to Beginning Android"
echo "[OK] Registered Nautilus context menu script"

# 2. KDE Dolphin ServiceMenu
KDE_SERVICES="${HOME}/.local/share/kio/servicemenus"
mkdir -p "${KDE_SERVICES}"

cat <<EOF > "${KDE_SERVICES}/beginning-android.desktop"
[Desktop Entry]
Type=Service
ServiceTypes=KonqPopupMenu/Plugin
MimeType=all/allfiles;
Actions=SendToBeginningAndroid;

[Desktop Action SendToBeginningAndroid]
Name=Send to Beginning Android
Icon=send-to
Exec=${INSTALL_DIR}/beginning-android send %F
EOF
echo "[OK] Registered KDE Dolphin ServiceMenu"

# 3. Standard Desktop Entry
APP_DIR="${HOME}/.local/share/applications"
mkdir -p "${APP_DIR}"

cat <<EOF > "${APP_DIR}/beginning-android.desktop"
[Desktop Entry]
Name=Beginning Android
Comment=Live clipboard and file sharing with Android
Exec=${INSTALL_DIR}/beginning-android
Terminal=false
Type=Application
Categories=Utility;Network;
EOF
echo "[OK] Created desktop application launcher"

echo "Installation complete! Beginning Android is registered for context menus."
