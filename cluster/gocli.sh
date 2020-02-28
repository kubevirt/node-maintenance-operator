#gocli_image="kubevirtci/gocli@sha256:2ff1e9cddfa2cfdf268301a52d1a5ec252ace6908196609e001844e5458b746a"
gocli_image="kubevirtci/gocli@sha256:e48c7285ac9e4e61fe0f89f35ac5f9090497ea7c8165deeadb61e464c88d8afd"
gocli="docker run --net=host --privileged --rm -v /var/run/docker.sock:/var/run/docker.sock $gocli_image"
gocli_interactive="docker run --net=host --privileged --rm -it -v /var/run/docker.sock:/var/run/docker.sock $gocli_image"
