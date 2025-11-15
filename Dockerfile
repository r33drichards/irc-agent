FROM golang

WORKDIR /go/src/app

# Install Deno
RUN apt-get update && \
    apt-get install -y curl unzip && \
    curl -fsSL https://deno.land/install.sh | sh && \
    ln -s /root/.deno/bin/deno /usr/local/bin/deno && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Copy deps.ts and cache AWS SDK dependencies
COPY deps.ts /tmp/deps.ts
RUN deno cache /tmp/deps.ts

# Copy and build Go application
COPY . .
RUN go mod tidy
RUN go build -o /go/bin/app -v .

CMD ["app"]