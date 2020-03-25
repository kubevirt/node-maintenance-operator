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
ACTION="unexpandleading"


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
    if [[ $1 != "" ]]; then
        echo "Error: $*"
    fi

cat <<EOF
$0 [-h] [-v] [-f] [-a expand|unexpand] [-t <tabstop>]

    -f                      : fix files if wrong (default report only)
    -v                      : verbose mode
    -h                      : show help message.
    -a <expand|unexpand|unexpandleading>  : specify the action (default $ACTION)
                            :    expand - convert tabs to spaces;
                            :    unexpand - convert spaces to tabs
                            :    unexpandleading - unexpand spaces before first token
    -t <tabstop>            : tabstop (default $TABSTOP)

fix or report tab/spaces issues in go files in current repository:
EOF
    exit 1
}


function buildunexpandleading {
    cat >unexpandleading.go  <<EOF
package main
import  (
    "fmt"
    "strconv"
    "strings"
    "bufio"
    "io"
    "os"
)

var tabStops = 4

func main() {
    fname := parseFlags()
    unexpandLeadingSpaces(fname)
}

func parseFlags() string {

    fname := ""
    for i := 1 ; i < len(os.Args); i+= 1 {
        if os.Args[i] == "-t" {
            if i + 1 < len(os.Args) {
                tabStops, _ = strconv.Atoi( os.Args[i+1] )
            }
            i+=1
        } else {
            fname = os.Args[i]
        }
    }

    if fname == "" ||  tabStops <= 0  {
        fmt.Printf("Usage:  %s [-t <tabstops>] <fname>",os.Args[0])
        os.Exit(1);
    }

    return fname

}

func unexpandLeadingSpaces(fn string) error {
    file, err := os.Open(fn)
    defer file.Close()

    if err != nil {
        return err
    }

    // Start reading from the file with a reader.
    reader := bufio.NewReader(file)

    var line string
    for {
        line, err = reader.ReadString('\n')

        unexpandLine(line)

        if err != nil {
            break
        }
    }

    if err != io.EOF {
        fmt.Printf(" > Failed!: %v\n", err)
    }

    return  nil
}

func unexpandLine(line string) {

    leadingSpacesLen := 0
    leadingSpacesPrefixLen := 0

    for i, c := range(line) {
        if c == ' ' {
            leadingSpacesLen += 1
        } else {
            if c == '\t' {
                leadingSpacesLen += tabStops
            } else {
                leadingSpacesPrefixLen = i
                break;
            }
        }
    }

    numTabs := leadingSpacesLen / tabStops
    numSpaces := leadingSpacesLen % tabStops

    outStr := strings.Repeat("\t", numTabs) + strings.Repeat(" ",numSpaces) + line[ leadingSpacesPrefixLen:len(line) ]
    fmt.Printf("%s", outStr)
}
EOF
    go build unexpandleading.go
    rm -f unexpandleading.go
}

function check_file {
    local FILE="$1"

    $ACTION -t $TABSTOP "$FILE" >$tmpfile

    if [[ $? != 0 ]]; then
        echo "can't process $FILE to $tmpfile error: $?"
        exit 1
    fi

    # also remove trailing spaces
    sed -i 's/[ \t]*$//' $tmpfile

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
            case $ACTION in
            expand)
                echo "$FILE has tabs. fix that with command: make tabs-fix"
                ;;
            unexpand)
                echo "$FILE has spaces. fix that with command: make tabs-fix"
                ;;
           ./unexpandleading)
                diff $tmpfile $FILE
                echo "$FILE has wrong leading spaces. fix that with command: make tabs-fix"
                ;;
            esac
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
        ((VERBOSE+=1))
        ;;
    *)
        Help "Invalid option"
        ;;
   esac
done

tmpfile=$(mktemp /tmp/tmpvim-enforce-spaces.XXXXX)


# --- the script ---

if [[ $VERBOSE == 2 ]]; then
   trace_on_total
fi

if [[ $ACTION != "expand" ]] && [[ $ACTION != "unexpand" ]] && [[ $ACTION != "unexpandleading" ]]; then
    echo "action should be either one of expand, unexpand, unexpandleading"
    Help "Invalid value of -f option"
fi

if [[ $ACTION == "unexpandleading" ]]; then
    buildunexpandleading
    ACTION="./unexpandleading"
fi

if [[ $ACTION == "unexpandleading" ]]; then
    buildunexpandleading
fi

for f in $(git ls-files -- ':!vendor/' | grep -E "*.go$" | grep -v "zz_generated.defaults.go"); do
    if [[ $VERBOSE == 1 ]]; then
        echo "check file: $f"
    fi

    check_file "$f"
done

if [[ $ACTION == "unexpandleading" ]]; then
    rm -f ./unexpandleading
fi

rm -f "$tmpfile"
