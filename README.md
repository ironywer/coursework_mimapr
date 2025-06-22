# coursework_mimapr

## Offline weights

The style transfer script relies on VGG19 weights. If the environment has no
internet access, set the `VGG_WEIGHTS` environment variable to the path of a
pre-downloaded `vgg19-dcbb9e9d.pth` file. When this variable is provided, the
weights will be loaded from disk instead of being downloaded.

When using `docker-compose`, place the weights file at `weights/vgg19-dcbb9e9d.pth`
in the project root. The compose configuration mounts this path and sets
`VGG_WEIGHTS` automatically for all nodes.
