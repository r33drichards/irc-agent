FROM golang

WORKDIR /go/src/app
COPY . .

# Install Deno
RUN apt-get update && apt-get install -y curl unzip && \
    curl -fsSL https://deno.land/install.sh | sh && \
    mv /root/.deno/bin/deno /usr/local/bin/ && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# Pre-cache deno-mcp package for faster startup
RUN deno cache jsr:@cong/mcp-deno

RUN go mod tidy
RUN go build -o /go/bin/app -v .

CMD ["app"]