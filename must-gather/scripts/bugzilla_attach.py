#!/bin/python3

"""
Parameters:

    id: The Bugzilla bug ID to send the attachment to
    image: The image to use (defaults to quay.io/kubevirt/must-gather)
    api-key: Use a generated API key instead of a username and login
    log-folder: Use a specific folder for storing the output of must-gather

Requirements:

    Openshift 4.1+
    Python 3.6+
    A Bugzilla account for www.bugzilla.redhat.com

This script attaches the result of the must-gather command, as executed
by the kubevirt must-gather image, to the supplied bugzilla id on the
Red Hat bugzilla website.
It first creates an output subdirectory in the working directory named
gather-files/ and then executes the following command:
'oc adm must-gather --image=quay.io/kubevirt/must-gather
--dest-dir=gather-files/' and pipes the output to
gather-files/must-gather.log
In order to meet the maximum attachment file sizes, logs are trimmed to the
last n seconds (defaulting to 30 minutes)
It then creates a time-stamped archive file to compress the attachment
and prepare it for upload.
After doing so, the attachment is encoded as a base64 string.
In order to authenticate against the Bugzilla website, a username and
password are prompted. A POST request is then sent to the Bugzilla
website. If there are any errors (invalid ID or invalid login), the
script prompts for those and retries the request until there are no
errors.
"""

import argparse
import os
import shutil
import itertools
import re
import subprocess
import tarfile
import datetime
import base64
from getpass import getpass
import requests

NUM_SECONDS = 30 * 60 # 30 minutes

BUGZILLA_URL = "https://bugzilla.redhat.com"

HEADERS = {'Content-type': 'application/json'}

LOGFOLDER = "gather-files/"

OUTPUT_FILE = "must-gather.log"

ARCHIVE_NAME = "must-gather"

MAX_ARCHIVE_SIZE = 19.5 * 1024 * 1024 #19.5 MB as bytes

IMAGE = "quay.io/kubevirt/must-gather"

NODELOG_TIMESTAMP_REGEX = re.compile(r"(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec) \d+ \d+:\d+:\d+")

NODELOG_TIMESTAMP_FORMAT = "%b %d %H:%M:%S"

PODLOG_TIMESTAMP_REGEX = re.compile(r"^\d{4}-\d{2}-\d{2}T\d+:\d+:\d+")

PODLOG_TIMESTAMP_FORMAT = "%Y-%m-%dT%H:%M:%S"

_current_time = datetime.datetime.utcnow()

def main():
    """Main function"""

    # Start with getting command-line argument(s)
    parser = argparse.ArgumentParser(description="Sends the result of must-gather to Bugzilla.")
    parser.add_argument("ID", metavar="id", type=int,
                        help="The ID of the bug in Bugzilla")
    parser.add_argument("--image", metavar="image", action="append",
                        help="The image to use for must-gather. If none supplied, defaults to quay.io/kubevirt/must-gather")
    parser.add_argument("--image-stream", metavar="image-stream", action="append",
                        help="Specify an image stream to pass to must-gather")
    parser.add_argument("--api-key", metavar="api-key",
                        help="Bugzilla API key. Can also be set using BUGZILLA_API_KEY environment variable")
    parser.add_argument("-t", "--time", type=int, help="Number of minutes to use for trimming the log files. Defaults to 30")
    parser.add_argument("--log-folder", metavar="log-folder",
                        help="Optional destination for the must-gather output (defaults to creating gather-files/ in the local directory)")
    parser.add_argument("-r", "--reuse-must-gather", action="store_true",
                        help="Use this to skip rerunning must-gather and just attach what is already gathered")
    parser.add_argument("-i", "--interactive", action="store_true",
                        help="Use this flag to prompt for a username and password. Using this will prompt for retries if the login is unsuccessful")
    args = parser.parse_args()

    bug_id = args.ID

    if not check_bug_exists(bug_id):
        print("Bug not found in Bugzilla")
        exit(1)

    if args.time:
        num_seconds = args.time * 60
    else:
        num_seconds = NUM_SECONDS

    # If an image or an image stream is supplied, use that, if not, use the default
    if args.image:
        images = args.image
    else:
        if args.image_stream == None:
            images = [IMAGE]
        else:
            images = []

    # If a folder is supplied, use that, otherwise use the default in the local folder
    if args.log_folder:
        logfolder = args.log_folder
    else:
        logfolder = LOGFOLDER

    api_key = os.environ.get('BUGZILLA_API_KEY', "")

    if args.api_key:
        api_key = args.api_key

    # If there is no API key provided, prompt for a login
    use_api_key = api_key != None and api_key != ""
    if not use_api_key:
        if args.interactive:
            bugzilla_username = input("Enter Bugzilla username: ")
            bugzilla_password = getpass(prompt="Enter Bugzilla password: ")
        else:
            print("No API key supplied and not in interactive mode.")
            exit(1)

    if not args.reuse_must_gather:
        run_must_gather(images, logfolder, args.image_stream)
    else:
        print("Using must-gather results located in %s." % logfolder)

    #Trim the log folders to the number of seconds
    trim_logs(logfolder, num_seconds)

    # Create a time-stamped archive name
    archive_name = ARCHIVE_NAME + "-%s.tar.gz" % _current_time.strftime("%Y-%m-%d_%H:%M:%SZ")

    # Add all the files in the log folder to a new archive file, except for the hidden ones
    with tarfile.open(archive_name, "w:gz") as tar:
        print("Creating archive: " + archive_name)
        tar.add(logfolder,
        filter=filter_hidden)

    # Now that the archive is created, move the files back in place of the trimmed versions
    restore_hidden_files(logfolder)

    if os.path.getsize(archive_name) > MAX_ARCHIVE_SIZE:
        print("Archive %s is too large to upload (exceeds %d MB)." % (archive_name, MAX_ARCHIVE_SIZE / (1024*1024)))
        exit()

    print("Preparing to send the data to " + BUGZILLA_URL)

    file_data = ""
    with open(archive_name, "rb") as data_file:
        file_data = base64.b64encode(data_file.read()).decode()

    comment = generate_comment(num_seconds)

    # Send the data to the target URL (depending on whether using API key or not)
    if use_api_key:
        authentication = {"api_key": api_key}
    else:
        authentication = {"username": bugzilla_username, "password:": bugzilla_password}
    resp = send_data(bug_id, archive_name, file_data, comment, authentication)
    resp_json = resp.json()

    # Handle the potential errors
    while "error" in resp_json:
        # Using an api key will disable retries, so just output the error message
        if use_api_key:
            print(resp_json["message"])
            exit(1)
        # 300: invalid username or password
        if resp_json["code"] == 300:
            print("Incorrect username or password.")
            bugzilla_username = input("Username (leave blank to exit): ")
            if bugzilla_username == "":
                print("Username left blank, exiting")
                exit(0)
            bugzilla_password = getpass(prompt="Password: ")
            authentication = {"username": bugzilla_username, "password:": bugzilla_password}
            resp = send_data(bug_id, archive_name, file_data, comment, authentication)
            resp_json = resp.json()
        # 101: Invalid bug id
        elif resp_json["code"] == 101:
            print("Invalid bug id")
            new_bug_id = input("Enter a new bug id (leave blank to exit): ")
            if new_bug_id == "":
                print("ID left blank, exiting")
                exit(0)
            bug_id, valid = try_parse_int(new_bug_id)
            # Try and see if the new supplied ID is a positive integer
            while not valid or bug_id <= 0:
                print("Could not parse bug id as valid, try again")
                new_bug_id = input("Enter a new bug id (leave blank to exit): ")
                if new_bug_id == "":
                    print("ID left blank, exiting")
                    exit(0)
                bug_id, valid = try_parse_int(new_bug_id)
            resp = send_data(bug_id, archive_name, file_data, comment, authentication)
            resp_json = resp.json()
        else:
            print("Error: " + resp_json["message"])
            exit(1)
    print("File successfully uploaded to Bugzilla")

def run_must_gather(images, logfolder, image_streams):
    # If the log folder already exists, delete it
    if os.path.isdir(logfolder):
        shutil.rmtree(logfolder)

    # Make a new log folder
    os.mkdir(logfolder)

    image_args = []
    if images is not None:
        for image in images:
            image_args.append("--image=" + image)
    if image_streams is not None:
        for image_stream in image_streams:
            image_args.append("--image-stream=" + image_stream)

    # Open the output file
    with open(logfolder + OUTPUT_FILE, "w+") as out_file:
        # Run oc adm must-gather with the appropriate image and dest-dir
        print("Running must-gather")
        try:
            subprocess.run(
                ["oc", "adm", "must-gather",
                "--dest-dir=" + logfolder] + image_args,
                stdout=out_file, stderr=subprocess.PIPE, check=True)
        except subprocess.CalledProcessError as e:
            print("Error in the execution of must-gather: ")
            print(e.stderr.decode("utf-8"))
            exit(1)

def trim_logs(logfolder, num_seconds):
    global _deadline
    _deadline = _current_time - datetime.timedelta(seconds = num_seconds)
    for subdir, _, files in os.walk(logfolder):
        for file in files:
            if file == "must-gather.log": #Ignore the log made by capturing the output of must-gather
                continue
            full_path = os.path.join(subdir, file)
            if ".log" in file:
                trim_from_back(full_path, pod_condition(full_path))
                #trim_file_by_time(os.path.join(subdir, file), num_seconds, PODLOG_TIMESTAMP_REGEX, PODLOG_TIMESTAMP_FORMAT)
            elif "kubelet" in file or "NetworkManager" in file:
                trim_from_back(full_path, node_condition(full_path))
                #trim_file_by_time(os.path.join(subdir, file), num_seconds, NODELOG_TIMESTAMP_REGEX, NODELOG_TIMESTAMP_FORMAT)

def try_parse_int(value):
    """Tries to parse the value as an int"""
    try:
        return int(value), True
    except ValueError:
        return value, False

def send_data(bug_id, file_name, file_data, comment, authentication):
    """Sends the data to Bugzilla with the relevant information"""
    url = BUGZILLA_URL + '/rest/bug/%s/attachment' % bug_id
    data = {
        **authentication,
        "ids": [bug_id],
        "comment": comment,
        "summary": "Result from must-gather command",
        "content_type": "application/gzip",
        "file_name": file_name,
        "data": file_data
    }
    return requests.post(url, json=data, headers=HEADERS)

"""Enum for the possible outputs of the line condition functions"""
LINE_LATER = 1
LINE_EARLIER = 0
LINE_NO_TIMESTAMP = -1

"""Regex and format for reading the header of a node log"""
HEADER_REGEX = re.compile(r"\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2} \w{3}")
HEADER_FORMAT = "%Y-%m-%d %H:%M:%S %Z"

def node_condition(filename):
    """Returns a line condition function based on the timestamp in the header"""
    with open(filename, "r") as file:
        header_timestamps = HEADER_REGEX.findall(file.readline())
        log_end = datetime.datetime.strptime(header_timestamps[1], HEADER_FORMAT)
    def check_line(line):
        #Empty string means end of file, otherwise it would be '\n'
        if line == '':
            return LINE_LATER
        regex_result = NODELOG_TIMESTAMP_REGEX.search(line)
        if regex_result:
            timestamp = datetime.datetime.strptime(regex_result.group(0), NODELOG_TIMESTAMP_FORMAT)

            #Since there's no year provided, default it to the log end's year
            timestamp = timestamp.replace(year=log_end.year)

            #If that made the timestamp greater than the log end, it means it was from a previous year, so set it to be the year before the log end
            if timestamp > log_end:
                timestamp = timestamp.replace(year=log_end.year - 1)

            # Check whether the line is earlier or later than the deadline
            if timestamp < _deadline:
                return LINE_EARLIER
            else:
                return LINE_LATER
        else:
            return LINE_NO_TIMESTAMP
    return check_line

def pod_condition(filename):
    """Returns a condition function for checking the lines of a pod log"""
    def check_line(line):
        #Empty string means end of file, otherwise it would be '\n'
        if line == '':
            return LINE_LATER
        regex_result = PODLOG_TIMESTAMP_REGEX.search(line)
        if regex_result:
            timestamp = datetime.datetime.strptime(regex_result.group(0), PODLOG_TIMESTAMP_FORMAT)
            if timestamp < _deadline:
                return LINE_EARLIER
            else:
                return LINE_LATER
        else:
            return LINE_NO_TIMESTAMP
    return check_line

"""Size of chunk to use for reading from the back of a log file."""
CHUNK_SIZE = 65536

def trim_from_back(filename, condition):
    """Scans chunks of data from the back of the file until it's reached a point that's earlier than the deadline.
    It then reads forward line by line until it reaches the correct point to trim."""
    print("Trimming %s" % filename)
    with open(filename, "r+") as file:
        file.seek(0, os.SEEK_END)
        curr_chunk_start = file.tell() - CHUNK_SIZE
        condition_result = LINE_LATER
        while curr_chunk_start > 0:
            file.seek(curr_chunk_start)
            file.readline() #Discard this since it's most likely a partial line
            condition_result = condition(file.readline()) #This is the first full line in the chunk
            while condition_result == LINE_NO_TIMESTAMP:
                condition_result = condition(file.readline()) # Read until there's a line that has a timestamp
            if condition_result == LINE_EARLIER:
                break
            curr_chunk_start -= CHUNK_SIZE
        #At this point the curr_chunk_start is either less than zero, or the chunk contains the first line later than the deadline
        if curr_chunk_start < 0:
            curr_chunk_start = 0
        file.seek(curr_chunk_start)
        trim_start = curr_chunk_start
        while condition_result != LINE_LATER:
            line = file.readline()
            trim_start += len(line)
            condition_result = condition(line)
        # trim_start is now right before the last line that was later than the deadline
        if trim_start == 0:
            return
        # Since this file will be trimmed, create a hidden copy of it
        hidden_filename = os.path.join(os.path.dirname(filename), "." + os.path.basename(filename))
        shutil.copy(filename, hidden_filename)
        file.seek(trim_start)
        # Read the data we want to keep
        content_to_keep = file.read()
        file.seek(0)
        file.truncate(0)
        file.write("This file was trimmed to only contain lines since %s\n" % _deadline.strftime("%Y-%m-%d %H:%M:%SZ"))
        file.write(content_to_keep)

def check_bug_exists(bug_id):
    """Checks whether the bug exists in Bugzilla"""
    url = BUGZILLA_URL + '/rest/bug/%s' % bug_id
    return "error" not in requests.get(url).json()

def generate_comment(num_seconds):
    """Creates the comment text for the attachment"""
    comment = ""
    comment += "Result from running must-gather"
    comment += "Log files were trimmed to the last %d" % num_seconds
    return comment

def filter_hidden(file):
    """Filters out hidden files so that the untrimmed ones won't be added to the archive"""
    return file if os.path.basename(os.path.normpath(file.name))[0] != "." else None

def restore_hidden_files(logfolder):
    """Finds any hidden files and renames them to their original name"""
    for subdir, _, files in os.walk(logfolder):
        for file in files:
            # If the file is hidden, it was a trimmed file so restore it
            if file[0] == ".":
                shutil.move(os.path.join(subdir, file), os.path.join(subdir, file[1:]))

main()
