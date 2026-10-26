#!/usr/bin/env bash
# Build the Android tablet host around the shared Go service and production UI.
# SDK/JDK and signing material are provisioned separately; this never installs tools.
set -euo pipefail
umask 077
cd "$(dirname "$0")/.."

usage() {
  cat <<'HELP'
Usage: bash packaging/build-android.sh [--build-ui]

Requires Go, gomobile/gobind, JDK, Python 3, an installed Android SDK platform 35,
build-tools 35.0.0 and NDK 27.2.12479018. By default frontend/dist must be built.
--build-ui runs npm's existing production build, without installing dependencies.

Environment:
  JAVA_HOME                         JDK installation (javac/java/jar)
  ANDROID_HOME / ANDROID_SDK_ROOT    Android SDK root
  ANDROID_NDK_HOME                  Optional installed NDK override
  CATALOGDB_ANDROID_KEYSTORE         Existing private signing keystore
  CATALOGDB_ANDROID_KEY_ALIAS        Signing alias
  CATALOGDB_ANDROID_STORE_PASS_FILE  File containing the keystore password
  CATALOGDB_ANDROID_KEY_PASS_FILE    File containing the key password

Signing keys/passwords must remain outside Git and must be backed up securely.
Reusing the same signing certificate is required for Android package updates.
HELP
}

build_ui=false
case "${1:-}" in
  '') ;;
  --build-ui) build_ui=true ;;
  -h|--help) usage; exit 0 ;;
  *) usage >&2; exit 2 ;;
esac
if (( $# > 1 )); then usage >&2; exit 2; fi
fail() { printf 'Android build: %s\n' "$*" >&2; exit 1; }
need_command() { command -v "$1" >/dev/null 2>&1 || fail "Missing $1; prepare the build environment first."; }
need_file() { [[ -f "$1" && -r "$1" ]] || fail "Required readable file missing: $1"; }

need_command python3
need_command go
need_command gomobile
need_command gobind
sdk_root="${ANDROID_HOME:-${ANDROID_SDK_ROOT:-}}"
[[ -n "$sdk_root" ]] || fail 'Set ANDROID_HOME or ANDROID_SDK_ROOT to the installed SDK.'
export ANDROID_HOME="$sdk_root"
export ANDROID_SDK_ROOT="$sdk_root"
export ANDROID_NDK_HOME="${ANDROID_NDK_HOME:-$sdk_root/ndk/27.2.12479018}"
[[ -d "$ANDROID_NDK_HOME/toolchains/llvm" ]] || fail 'Install NDK 27.2.12479018 or set ANDROID_NDK_HOME.'
if [[ -n "${JAVA_HOME:-}" ]]; then
  java_bin="$JAVA_HOME/bin/java"
  javac_bin="$JAVA_HOME/bin/javac"
  jar_bin="$JAVA_HOME/bin/jar"
  export PATH="$JAVA_HOME/bin:$PATH"
else
  need_command java
  need_command javac
  need_command jar
  java_bin=$(command -v java)
  javac_bin=$(command -v javac)
  jar_bin=$(command -v jar)
fi
[[ -x "$java_bin" && -x "$javac_bin" && -x "$jar_bin" ]] || fail 'JAVA_HOME must point to a JDK, not a JRE.'

tools="$sdk_root/build-tools/${CATALOGDB_ANDROID_BUILD_TOOLS:-35.0.0}"
android_jar="$sdk_root/platforms/android-35/android.jar"
need_file "$android_jar"
for tool in aapt2 d8 zipalign apksigner; do
  [[ -x "$tools/$tool" ]] || fail "Missing Android build-tool $tool in $tools"
done

: "${CATALOGDB_ANDROID_KEYSTORE:?Set the private Android signing keystore path}"
: "${CATALOGDB_ANDROID_KEY_ALIAS:?Set the signing key alias}"
: "${CATALOGDB_ANDROID_STORE_PASS_FILE:?Set the keystore password file path}"
: "${CATALOGDB_ANDROID_KEY_PASS_FILE:?Set the private key password file path}"
need_file "$CATALOGDB_ANDROID_KEYSTORE"
need_file "$CATALOGDB_ANDROID_STORE_PASS_FILE"
need_file "$CATALOGDB_ANDROID_KEY_PASS_FILE"
if "$build_ui"; then npm --prefix frontend run build; fi
need_file frontend/dist/index.html

version=$(python3 - <<'PY'
import json,re,xml.etree.ElementTree as ET
from pathlib import Path
version=json.loads(Path('wails.json').read_text())['info']['productVersion']
if not re.fullmatch(r'\d+\.\d+\.\d+',version):raise SystemExit('Invalid application version')
manifest=ET.parse('android/AndroidManifest.xml').getroot()
ns='{http://schemas.android.com/apk/res/android}'
if manifest.attrib.get(ns+'versionName')!=version:raise SystemExit('Android and application versions differ')
sdk=manifest.find('uses-sdk')
if sdk is None or sdk.attrib.get(ns+'minSdkVersion')!='26' or sdk.attrib.get(ns+'targetSdkVersion')!='35':raise SystemExit('Android SDK levels differ from build configuration')
app=manifest.find('application')
if app is None or app.attrib.get(ns+'allowBackup')!='false' or app.attrib.get(ns+'debuggable','false')!='false':raise SystemExit('Unsafe Android release manifest')
print(version)
PY
)

mkdir -p build/android build/bin
work=$(mktemp -d "$PWD/build/android/package.XXXXXX")
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/aar" "$work/generated" "$work/classes" "$work/dex"
# gomobile builds generated JNI sources outside the repository; no useful VCS
# metadata can be recovered there (desktop Wails builds disable it as well).
export GOFLAGS="${GOFLAGS:-} -buildvcs=false"
printf 'Building shared Go service for Android (arm, arm64, amd64)...\n'
gomobile bind -target=android/arm,android/arm64,android/amd64 -androidapi 26 \
  -ldflags='-s -w -extldflags=-Wl,-z,max-page-size=16384' \
  -o "$work/catalogdb.aar" ./mobile

python3 - "$work" <<'PY'
from pathlib import Path
import struct,sys,zipfile
work=Path(sys.argv[1])
expected={'armeabi-v7a','arm64-v8a','x86_64'}
with zipfile.ZipFile(work/'catalogdb.aar') as archive:
    members=archive.namelist()
    required=['classes.jar']+[f'jni/{abi}/libgojni.so' for abi in sorted(expected)]
    for name in required:
        if name not in members:raise SystemExit(f'AAR is missing {name}')
        target=work/'aar'/name;target.parent.mkdir(parents=True,exist_ok=True)
        payload=archive.read(name);target.write_bytes(payload)
        if name.endswith('.so'):
            if payload[:4]!=b'\x7fELF' or payload[5]!=1:raise SystemExit(f'Unexpected ELF: {name}')
            elf64=payload[4]==2
            off=struct.unpack_from('<Q' if elf64 else '<I',payload,32 if elf64 else 28)[0]
            entry,count=struct.unpack_from('<HH',payload,54 if elf64 else 42)
            loads=0
            for pos in range(off,off+entry*count,entry):
                if struct.unpack_from('<I',payload,pos)[0]!=1:continue
                loads+=1
                fileoff=struct.unpack_from('<Q' if elf64 else '<I',payload,pos+(8 if elf64 else 4))[0]
                address=struct.unpack_from('<Q' if elf64 else '<I',payload,pos+(16 if elf64 else 8))[0]
                align=struct.unpack_from('<Q' if elf64 else '<I',payload,pos+(48 if elf64 else 28))[0]
                if align<16384 or (address-fileoff)%16384:raise SystemExit(f'ELF LOAD is not 16 KiB aligned: {name}')
            if not loads:raise SystemExit(f'ELF has no loadable segments: {name}')
            print(f'Verified ELF alignment: {name}')
PY

printf 'Compiling native Android resources and Java host...\n'
"$tools/aapt2" compile --dir android/res -o "$work/resources.zip"
"$tools/aapt2" link -o "$work/resources.apk" --manifest android/AndroidManifest.xml \
  -I "$android_jar" --java "$work/generated" -0 arsc "$work/resources.zip"
python3 - "$work" <<'PY'
from pathlib import Path
import sys
work=Path(sys.argv[1])
files=sorted(Path('android/src').rglob('*.java'))+sorted((work/'generated').rglob('*.java'))
if not files:raise SystemExit('Android Java sources are missing')
# javac argument files use double-quoted paths; reject ambiguous path characters.
if any(any(c in str(path) for c in '\n\r"\\') for path in files):raise SystemExit('Unsupported Java source path')
(work/'sources.txt').write_text('\n'.join('"'+str(path)+'"' for path in files)+'\n')
PY
# Keep the JDK bootstrap classes available for javac's lambda generation. D8
# desugars the Java 8 bytecode against the Android platform below.
"$javac_bin" -encoding UTF-8 -source 8 -target 8 -Xlint:-options \
  -classpath "$android_jar:$work/aar/classes.jar" \
  -d "$work/classes" @"$work/sources.txt"
"$jar_bin" cf "$work/host.jar" -C "$work/classes" .
"$tools/d8" --lib "$android_jar" --min-api 26 --output "$work/dex" \
  "$work/host.jar" "$work/aar/classes.jar"

python3 - "$work" <<'PY'
from pathlib import Path
import sys,zipfile
work=Path(sys.argv[1]);frontend=Path('frontend/dist')
if not list((work/'dex').glob('classes*.dex')):raise SystemExit('D8 did not produce classes.dex')
with zipfile.ZipFile(work/'unsigned.apk','x') as out:
    with zipfile.ZipFile(work/'resources.apk') as resources:
        for info in resources.infolist():
            compression=zipfile.ZIP_STORED if info.filename=='resources.arsc' else info.compress_type
            out.writestr(info.filename,resources.read(info),compress_type=compression)
    for source in sorted((work/'dex').glob('classes*.dex')):
        out.write(source,source.name,compress_type=zipfile.ZIP_DEFLATED)
    for source in sorted(frontend.rglob('*')):
        if source.is_symlink():raise SystemExit('Frontend assets cannot contain symlinks')
        if source.is_file():out.write(source,'assets/web/'+source.relative_to(frontend).as_posix(),compress_type=zipfile.ZIP_DEFLATED)
    for source in sorted((work/'aar/jni').glob('*/libgojni.so')):
        out.write(source,'lib/'+source.parent.name+'/'+source.name,compress_type=zipfile.ZIP_STORED)
    out.write('docs/pdf-notices.txt','assets/catalogdb/pdf-notices.txt',compress_type=zipfile.ZIP_DEFLATED)
with zipfile.ZipFile(work/'unsigned.apk') as archive:
    if archive.testzip() is not None:raise SystemExit('APK ZIP is corrupt')
    names=archive.namelist()
    if len(names)!=len(set(names)):raise SystemExit('Duplicate APK members')
    if not {'AndroidManifest.xml','resources.arsc','classes.dex','assets/web/index.html'}.issubset(names):raise SystemExit('APK contents are incomplete')
    if any(name.startswith('/') or '..' in name.split('/') for name in names):raise SystemExit('Unsafe APK member path')
    forbidden={'.env','connection.json','release.keystore','debug.keystore'}
    if any(Path(name).name in forbidden or Path(name).suffix.lower() in {'.jks','.keystore','.p12'} for name in names):raise SystemExit('Private build material found in APK')
    print('Verified packaged UI, DEX, native libraries and safe member paths.')
PY

"$tools/zipalign" -P 16 -f 4 "$work/unsigned.apk" "$work/aligned.apk"
output="$PWD/build/bin/CatalogDB-android-$version.apk"
# apksigner consumes successive lines when the same password-file path is used
# twice. Separate private copies also support PKCS12 with identical passwords.
install -m 600 "$CATALOGDB_ANDROID_STORE_PASS_FILE" "$work/store-password"
install -m 600 "$CATALOGDB_ANDROID_KEY_PASS_FILE" "$work/key-password"
"$tools/apksigner" sign --ks "$CATALOGDB_ANDROID_KEYSTORE" \
  --ks-key-alias "$CATALOGDB_ANDROID_KEY_ALIAS" \
  --ks-pass "file:$work/store-password" \
  --key-pass "file:$work/key-password" \
  --v1-signer-name CERT \
  --v1-signing-enabled true --v2-signing-enabled true --v3-signing-enabled false \
  --v4-signing-enabled false \
  --out "$work/signed.apk" "$work/aligned.apk"
"$tools/apksigner" verify --verbose --print-certs "$work/signed.apk"
# Modern verification skips v1 when v2 is present. Check the JAR signature
# independently as well, since vendor installers may inspect it first.
"$tools/apksigner" verify --verbose --min-sdk-version 21 --max-sdk-version 23 "$work/signed.apk"
python3 - "$work/signed.apk" <<'PY'
import sys,zipfile
with zipfile.ZipFile(sys.argv[1]) as archive:
    required={'META-INF/CERT.RSA','META-INF/CERT.SF','META-INF/MANIFEST.MF'}
    if not required.issubset(archive.namelist()):raise SystemExit('Standard v1 certificate entries missing')
PY
"$tools/zipalign" -c -P 16 4 "$work/signed.apk"
install -m 644 "$work/signed.apk" "$output"
python3 - "$output" <<'PY'
from pathlib import Path
import hashlib,sys
path=Path(sys.argv[1]);digest=hashlib.sha256(path.read_bytes()).hexdigest()
Path(str(path)+'.sha256').write_text(digest+'  '+path.name+'\n')
print(f'Built {path} ({path.stat().st_size} bytes)')
print(f'SHA-256 {digest}')
PY
