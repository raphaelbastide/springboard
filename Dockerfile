FROM alpine

ADD build/linux-amd64/springboard /usr/local/bin/springboard

CMD /usr/local/bin/springboard serve