package main

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Microsoft/hcsshim/ext4/tar2ext4"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

const (
	// Directory where the Docker image manifest will be extracted
	WorkDir = "work"
	// Directory where the Docker image layers will be extracted
	ExtractionDir = "extraction"
	// Directory where the target Dockerfile is located
	DataDir = "data"
	// InvalidImage is the default value for the -tag flag
	InvalidImage = "@@@ INVALID IMAGE @@@"
)

func main() {
	tag_flag := flag.String("image", InvalidImage, "Image to convert")
	tag_data_dir := flag.String("data-dir", DataDir, "Directory where the target Dockerfile is located")

	flag.Parse()

	tag := *tag_flag
	tag_file := dockerTagToValidFileName(tag)

	if tag == InvalidImage {
		panic("invalid / no image tag specified, use -image please!")
	}

	work_tar := tag_file + ".image.tar"

	out_tar := tag_file + ".tar"
	out_ext4 := tag_file + ".ext4"
	out_vhd := tag_file + ".ext4.vhd"
	out_overlayfs := tag_file + ".overlayfs.ext4"

	ctx := context.Background()
	// Run setup (mkdir -p etc.) and get a Docker client
	cli := setup()

	// Store pwd for later
	pwd, err := os.Getwd()
	golangErrorHandling(err)

	// chdir into data/ and build
	chdir(*tag_data_dir)
	buildImageFromDockerfile(cli, ctx, []string{tag})
	chdir(pwd)

	// Save the image to pwd
	saveImageAsTar(cli, ctx, tag, work_tar)

	// cd into work/ and extract the tarball
	extractImageToWorkDir(cli, ctx, work_tar)
	chdir(pwd)

	// Create a tarball from the extraction directory
	out_tar_writer, err := os.Create(out_tar)
	golangErrorHandling(err)
	defer out_tar_writer.Close()
	chdir(filepath.Join(WorkDir, ExtractionDir))
	createTarball(".", out_tar_writer)
	os.Chdir(pwd)

	// Convert the tarball to an ext4 image
	tarballToExt4(out_tar, out_ext4)
	// Convert the tarball to a VHD-compatible ext4 image
	tarballToExt4(out_tar, out_vhd, tar2ext4.AppendVhdFooter)
	// Convert the tarball to an OverlayFS-compatible ext4 image
	tarballToExt4(out_tar, out_overlayfs, tar2ext4.ConvertWhiteout)

	fmt.Println("done :sparkles:")
}

func buildImageFromDockerfile(cli *client.Client, ctx context.Context, tags []string) {
	buildCtx := new(bytes.Buffer)
	createTarball("./", buildCtx)

	resp, err := cli.ImageBuild(ctx, buildCtx, types.ImageBuildOptions{Tags: tags})
	golangErrorHandling(err)
	defer resp.Body.Close()

	_, err = io.Copy(os.Stdout, resp.Body)
	golangErrorHandling(err)
}

func saveImageAsTar(cli *client.Client, ctx context.Context, image string, tarball string) {
	stream, err := cli.ImageSave(ctx, []string{image})
	golangErrorHandling(err)
	defer stream.Close()

	out_work_tar_writer, err := os.Create(tarball)
	golangErrorHandling(err)
	defer out_work_tar_writer.Close()

	_, err = io.Copy(out_work_tar_writer, stream)
	golangErrorHandling(err)
}

func extractImageToWorkDir(cli *client.Client, ctx context.Context, work_tar string) {
	in_work_tar_reader, err := os.Open(work_tar)
	golangErrorHandling(err)
	defer in_work_tar_reader.Close()

	chdir(WorkDir)

	err = untar(".", in_work_tar_reader)
	golangErrorHandling(err)

	manifest_file := "manifest.json"
	golangErrorHandling(err)
	manifest_json, err := os.ReadFile(manifest_file)
	golangErrorHandling(err)

	var result []map[string]interface{}
	err = json.Unmarshal(manifest_json, &result)
	golangErrorHandling(err)

	layers := result[0]["Layers"]

	err = os.Chdir(ExtractionDir)
	golangErrorHandling(err)
	// Extract every layer into the extraction dir
	for _, layer := range layers.([]interface{}) {
		layer_file := layer.(string)
		fmt.Println(layer_file)

		untar_wd, err := os.Getwd()
		golangErrorHandling(err)

		in_layer_tar_reader, err := os.Open(filepath.Join("..", layer_file))
		golangErrorHandling(err)
		defer in_layer_tar_reader.Close()

		err = untar(untar_wd, in_layer_tar_reader)
		golangErrorHandling(err)
	}
}

func createTarball(from_path string, out_tar_writer io.Writer) {
	tw := tar.NewWriter(out_tar_writer)
	defer tw.Close()

	err := filepath.Walk(from_path, func(path string, info os.FileInfo, err error) error {
		golangErrorHandling(err)

		fmt.Println("tarring " + path)

		header, err := tar.FileInfoHeader(info, info.Name())
		golangErrorHandling(err)

		header.Name = path

		err = tw.WriteHeader(header)
		golangErrorHandling(err)

		if !info.Mode().IsRegular() {
			return nil
		}

		f, err := os.Open(path)
		golangErrorHandling(err)
		defer f.Close()

		_, err = io.Copy(tw, f)
		golangErrorHandling(err)

		return nil
	})
	golangErrorHandling(err)
}

func tarballToExt4(out_tar string, out_ext4 string, opts ...tar2ext4.Option) {
	out_ext4_writer, err := os.Create(out_ext4)
	golangErrorHandling(err)
	defer out_ext4_writer.Close()

	in_tar_reader, err := os.Open(out_tar)
	golangErrorHandling(err)
	defer in_tar_reader.Close()

	err = tar2ext4.Convert(in_tar_reader, out_ext4_writer, opts...)
	golangErrorHandling(err)
}

func dockerTagToValidFileName(tag string) string {
	return strings.Replace(tag, "/", "_", -1)
}

func chdir(dir string) {
	err := os.Chdir(dir)
	golangErrorHandling(err)
}

func setup() *client.Client {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	golangErrorHandling(err)

	err = os.MkdirAll(filepath.Join(WorkDir, ExtractionDir), 0755)
	golangErrorHandling(err)

	return cli
}

func golangErrorHandling(err error) {
	if err != nil {
		panic(err)
	}
}

// https://medium.com/@skdomino/taring-untaring-files-in-go-6b07cf56bc07
// adapted to not gzip
func untar(dst string, r io.Reader) error {
	tr := tar.NewReader(r)

	for {
		header, err := tr.Next()

		switch {
		case err == io.EOF:
			return nil

		case err != nil:
			return err

		case header == nil:
			continue
		}

		target := filepath.Join(dst, header.Name)

		fmt.Println("untar " + target)

		// the following switch could also be done using fi.Mode(), not sure if there
		// a benefit of using one vs. the other.
		// fi := header.FileInfo()

		// check the file type
		switch header.Typeflag {

		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0755); err != nil {
					return err
				}
			}

		case tar.TypeReg:
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}

			if _, err := io.Copy(f, tr); err != nil {
				return err
			}

			f.Close()
		}
	}
}
