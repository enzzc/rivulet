FROM alpine:3.15
ADD rivulet rivulet
ADD index.tpl index.tpl
ENTRYPOINT ["./rivulet"]
