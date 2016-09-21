# Nodeblob

Nodeblob caches node_modules directories on S3.

It calculates a hash based off the declared dependencies of the project and uses this to construct an S3 object key. As long as the object exists (nodeblob has run before and the dependencies are unchanged) it will download and use the cached modules. If we get a cache miss it will run `npm install` as usual and upload the resulting cache archive.

## Usage

`nodeblob -bucket "name-of-bucket" [-prefix "cache key prefix"] [<project path>]`

Nodeblob expects a [configured AWS CLI environment](http://docs.aws.amazon.com/cli/latest/userguide/cli-chap-getting-started.html), you might have to set the `AWS_REGION` environment variable to the region of your bucket depending on your config.
