FROM node:6.9.1

RUN apt-get update && apt-get install -y --no-install-recommends \
	jq \
	&& rm -rf /var/lib/apt/lists/*

# Add nodeblob binary
ADD ./bin/nodeblob /usr/local/bin/nodeblob

