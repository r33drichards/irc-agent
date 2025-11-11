FROM golang

WORKDIR /go/src/app
COPY . .

RUN go mod init app && go mod tidy
RUN go install -v ./...

CMD ["app"]