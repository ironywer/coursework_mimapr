# coursework_mimapr

## Offline weights

The style transfer script relies on VGG19 weights. If the environment has no
internet access, set the `VGG_WEIGHTS` environment variable to the path of a
pre-downloaded `vgg19-dcbb9e9d.pth` file. When this variable is provided, the
weights will be loaded from disk instead of being downloaded.
