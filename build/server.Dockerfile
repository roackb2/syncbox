FROM go-base

RUN mkdir -p /go/src/github.com/roackb2/syncbox
WORKDIR /go/src/github.com/roackb2/syncbox

COPY . /go/src/github.com/roackb2/syncbox
RUN go build
RUN go install ./sb-server
CMD ["sb-server"]
