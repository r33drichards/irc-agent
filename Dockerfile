FROM golang

WORKDIR /go/src/app

# Install Deno
RUN apt-get update && \
    apt-get install -y curl unzip && \
    curl -fsSL https://deno.land/install.sh | sh && \
    mv /root/.deno/bin/deno /usr/local/bin/deno && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

COPY . .

RUN go mod tidy
RUN go build -o /go/bin/app -v .

CMD ["app"]