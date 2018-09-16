FROM golang:alpine as builder

#install git
RUN apk add --update --no-cache git

#add local src folder to image src folder
ADD . $GOPATH/src/splitwiseExpenseAPI/

#first install splitwiseExpenseAPI
WORKDIR $GOPATH/src/splitwiseExpenseAPI/

#get dependencies
RUN go get ./...

#run build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -ldflags="-w -s" -o /go/bin/splitwiseExpenseAPI

#build a small image
#FROM scratch
FROM alpine

#install ca certificates necessary for smtp
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/


#copy static exec
WORKDIR /go/bin
COPY --from=builder /go/bin/splitwiseExpenseAPI .

#copy static html and config files
#COPY --from=builder /go/src/splitwiseExpenseAPI/view ./view
#COPY --from=builder /go/src/splitwiseExpenseAPI/config.json ./splitwiseconfig.json


#entrypoint
ENTRYPOINT ["/go/bin/splitwiseExpenseAPI","-config=/data/splitwiseconfig.json","-log=/data/splitwiseExpenseAPIServer.log"]
#expose port
EXPOSE 9093

#Build
#sudo docker build -t splitwiseexpenseapi1.0 . 

#rm
#sudo docker rm splitwiseExpenseAPI

#run splitwiseExpenseAPI
#sudo docker run -p 9093:9093 --name splitwiseExpenseAPI -v ~/work/gocode/src/data:/data splitwiseexpenseapi1.0 