#!/bin/bash

#
# 1 - fix automatically if wrong
# 0 - report and exit with error
#
FIX_IF_WRONG=0

#
# number of tab stops (used when errors are fixed with expand)
#
TABSTOP=4

#
# expand - convert tabs to spaces
# unexpand - convert spaces to tabs.
#
#
ACTION="unexpand"


#declare -A explain=( ["expand"]="convert tabs to spaces" ["unexpand"]="convert spaces to tabs" )

VERBOSE=0


function trace_on_total
{
    SCRIPT_TRACE_ON=1
    OLD_PS4=$PS4
    export PS4='+(${BASH_SOURCE}:${LINENO}): ${FUNCNAME[0]:+${FUNCNAME[0]}(): }'
    set -x
}

function Help {
cat <<EOF
$0 [-h] [-v] [-f] [-a expand|unexpand] [-t <tabstop>]

    -f                      : fix files if wrong (default report only)
    -v                      : verbose mode
    -h                      : show help message.
    -a <expand|unexpand>    : action: expan - convert tabs to spaces; unexpand - convert spaces to tabs (default $ACTION)
    -t <tabstop>            : tabstop (default $TABSTOP)

fix or report tab/spaces issues in go files in current repository:
EOF
    exit 1
}

if [[ $VERBOSE == 2 ]]; then
   trace_on_total
fi

while getopts "hfva:t:" opt; do
  case ${opt} in
    h)
        Help
        ;;
    a)
        ACTION="$OPTARG"
        ;;
    f)
        FIX_IF_WRONG=1
        ;;
    t)
        TABSTOP="$OPTARG"
        ;;
    v)
        set -x
        export PS4='+(${BASH_SOURCE}:${LINENO})'
        VERBOSE=1
        ;;
    *)
        Help "Invalid option"
        ;;
   esac
done

if [[ $ACTION != "expand" ]] && [[ $ACTION != "unexpand" ]]; then
    echo "action should be either expand or unexpand"
    Help "Invalid value of -f option"
fi


tmpfile=$(mktemp /tmp/tmpvim-enforce-spaces.XXXXX)

function check_file {
    local FILE="$1"

    $ACTION -t $TABSTOP "$FILE" >$tmpfile
    sed -i 's/[ \t]*$//' $tmpfile

    if [[ $? != 0 ]]; then
        echo "can't copy $FILE to $tmpfile error: $?"
        exit 1
    fi


    diff $tmpfile $FILE >/dev/null

    stat=$?

    if [[ $stat == 2 ]]; then
        echo "failed to compare $tmpfile and $FILE"
        exit 1
    fi

    if [[ $stat == 1 ]]; then
        if [[ $FIX_IF_WRONG == 1 ]]; then

            echo "fix file $FILE apply command: $ACTION $FILE"
            cp -f "$tmpfile" "$FILE"
            if [ $? != 0 ]; then
                echo "failed to copy $tmpfile to $FILE error: $?"
                exit 1
            fi
        else
            if [ $ACTION == "expand" ]; then
                echo "$FILE has tabs. fix that with command: $ACTION $FILE >tmpfile; mv -f tmpfile $FILE"

            else
                echo "$FILE has spaces. fix that with command: $ACTION $FILE >tmpfile; mv -f tmpfile $FILE"
            fi
            exit 1
        fi
    elif [[ $stat == 0 ]]; then
       if [[ $VERBOSE == 1 ]]; then
            echo "ok"
       fi
    else
       echo "unexpected status: $?"
       exit 1
    fi

}


for f in $(git ls-files -- ':!vendor/' | grep -E "*.go$"); do
    if [[ $VERBOSE == 1 ]]; then
        echo "check file: $f"
    fi

    check_file "$f"
done

rm -f "$tmpfile"
