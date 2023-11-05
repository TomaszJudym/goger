FROM selenium/standalone-chrome:114.0-chromedriver-114.0

USER root

RUN apt update \
    && apt install -y python3-pip
 
# send logs straight to terminal
ENV PYTHONUNBUFFERED 1
# set display port to avoid crash
ENV DISPLAY=:99

# INSTALL PYTHON DEPS
COPY requirements.txt get-gog-games.py /app/
RUN pip3 install -r /app/requirements.txt

CMD ["python3", "/app/get-gog-games.py"]
