# Rootless

## Requirements

Being able to run the rootless containers may require `sudo` access in some OSs to enable unprivileged containers 
support (Arch or Debian for example), as documented, for example, in the 
[buildah documentation](https://wiki.archlinux.org/index.php/Buildah#Enable_support_to_build_unprivileged_containers) 
or in the [usernetes documentation](https://github.com/rootless-containers/usernetes#distribution-specific-hint):

```sh
# Only for the current session
sudo sysctl kernel.unprivileged_userns_clone=1
```

```sh
# Enable the permission permanently
echo "kernel.unprivileged_userns_clone=1" >> /etc/sysctl.conf
sudo sysctl -p
```

## Run bblfshd in non-privileged mode

As documented in the [Docker docs](https://docs.docker.com/engine/security/seccomp/), 
the default security profile disables commands such as `unshare`, `mount` or `sethostname` inside containers 
(which are needed for example to spawn containers inside `bblfshd` and also to give a `Hostname` to each 
container to be an identified driver). Also, as documented in 
[libcontainer#1658](https://github.com/opencontainers/runc/issues/1658), 
there is a known bug with rootless containers inside another non-root container and the `/proc` mount / masking. 
Adding a volume `-v /proc:/newproc` would solve that problem. 

Therefore to run `bblfshd` in non privileged mode, this would suffice:


```sh
docker run --name bblfshd \
  -p 9432:9432 \
  -v /var/lib/bblfshd:/var/lib/bblfshd \
  -v /proc:/newproc \
  --security-opt seccomp=unconfined \
  bblfshd
```

A better (and recommended) confinement configuration, would be:

```sh
docker run --name bblfshd \
  -p 9432:9432 \
  -v /var/lib/bblfshd:/var/lib/bblfshd \
  -v /proc:/newproc \
  --security-opt seccomp=./bblfshd-seccomp.json \
  bblfshd
```

[`./bblfshd-seccomp.json`](./bblfshd-seccomp.json) file is a modification of 
[`default.json`](https://github.com/moby/moby/blob/master/profiles/seccomp/default.json) from Docker which allows 
the following syscalls inside `bblfshd` container: `mount, unshare, pivot_root, keyctl, umount2, sethostname`.

