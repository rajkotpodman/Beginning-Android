FROM python:3.11-alpine

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY server.py .

ENV PORT=8080
ENV HOST=0.0.0.0

EXPOSE 8080

CMD ["python", "server.py"]
