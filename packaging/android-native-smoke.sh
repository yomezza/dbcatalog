#!/usr/bin/env bash
# Device integration against compose PostgreSQL ONLY. The separate test APK is
# signed with the app certificate and can instrument that app; never publish it.
set -euo pipefail
umask 077
cd "$(dirname "$0")/.."
schema=${1:?Pass a fresh android_smoke_NAME schema (disposable compose DB only)}
phase=${2:-create}
[[ "$schema" =~ ^android_smoke_[a-z0-9_]{1,40}$ && "$phase" =~ ^(create|resume)$ ]]
: "${ANDROID_HOME:?}" "${JAVA_HOME:?}" "${CATALOGDB_ANDROID_KEYSTORE:?}" \
  "${CATALOGDB_ANDROID_KEY_ALIAS:?}" "${CATALOGDB_ANDROID_STORE_PASS_FILE:?}" \
  "${CATALOGDB_ANDROID_KEY_PASS_FILE:?}"
tools="$ANDROID_HOME/build-tools/35.0.0"
platform="$ANDROID_HOME/platforms/android-35/android.jar"
adb="$ANDROID_HOME/platform-tools/adb"
mkdir -p build/android
work=$(mktemp -d "$PWD/build/android/native-smoke.XXXXXX")
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/classes" "$work/dex"
"$tools/aapt2" link -o "$work/test.apk" --manifest android/tests/AndroidManifest.xml -I "$platform"
"$JAVA_HOME/bin/javac" -encoding UTF-8 -source 8 -target 8 -Xlint:-options \
  -classpath "$platform" -d "$work/classes" android/tests/src/ru/dbcatalog/smoke/NativeSmoke.java
"$JAVA_HOME/bin/jar" cf "$work/tests.jar" -C "$work/classes" .
"$tools/d8" --lib "$platform" --min-api 26 --output "$work/dex" "$work/tests.jar"
python3 - "$work" <<'PY'
from pathlib import Path
import sys,zipfile
work=Path(sys.argv[1])
with zipfile.ZipFile(work/'test.apk','a') as archive:
    archive.write(work/'dex/classes.dex','classes.dex')
PY
"$tools/zipalign" -f 4 "$work/test.apk" "$work/aligned.apk"
install -m 600 "$CATALOGDB_ANDROID_STORE_PASS_FILE" "$work/store-password"
install -m 600 "$CATALOGDB_ANDROID_KEY_PASS_FILE" "$work/key-password"
"$tools/apksigner" sign --ks "$CATALOGDB_ANDROID_KEYSTORE" --ks-key-alias "$CATALOGDB_ANDROID_KEY_ALIAS" \
  --ks-pass "file:$work/store-password" --key-pass "file:$work/key-password" \
  --out "$work/signed.apk" "$work/aligned.apk"
"$adb" install --no-incremental -r "$work/signed.apk"
"$adb" shell am instrument -w -e schema "$schema" -e phase "$phase" \
  ru.dbcatalog.smoke/ru.dbcatalog.smoke.NativeSmoke | tee "$work/result.txt"
grep -q '^PASS: Android' "$work/result.txt"
