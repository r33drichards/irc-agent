FROM golang

WORKDIR /go/src/app
COPY . .

RUN go mod tidy
RUN go install -v ./...

CMD ["app"]