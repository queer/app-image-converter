# converter

A little CLI that converts Dockerfiles. Takes in a directory containing a
Dockerfile and necessary files for building, and produces:

- A Docker image on your system
- A tarball of the Docker image export (`docker save ...`)
- A tarball of the filesystem of the Docker image
- Multiple ext4 images of the Docker image's filesystem:
  - Plain ext4
  - OverlayFS-compatible ext4
  - VHD-compatible EXT4

## Building

```bash
$ go build .
# now copy ./converter somewhere
```

## Usage

```bash
$ ./converter --image hello-world
[...]
$ ls
[...]
# ext4 image
hello-world.ext4
# VHD-compatible ext4 image
hello-world.ext4.vhd
# Output of docker save ...
hello-world.image.tar
# OverlayFS-compatible ext4 image
hello-world.overlayfs.ext4
# Flat tarball of the image contents
hello-world.tar
[...]
```

## TODO (maybe)

- Buildpack support: https://pkg.go.dev/github.com/buildpacks/lifecycle#Builder
- Nixpacks support?: https://github.com/railwayapp/nixpacks