FROM golang:1.6

RUN apt-get update
RUN apt-get install -y net-tools net-tools wget
