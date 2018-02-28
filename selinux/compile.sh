#!/bin/sh

checkmodule -M -m -o bblfshd.mod bblfshd.te && \
    semodule_package -o bblfshd.pp -m bblfshd.mod && \
    echo 'Module compiled, load it with semodule -i bblfshd.pp'

# enable with: semodule -i bblfshd.pp
