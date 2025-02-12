# Python rasmidan foydalanamiz
FROM python:3.12-slim

# Ishchi katalogni yaratish
WORKDIR /app

# Talablar faylini konteynerga nusxalash
COPY requirements.txt .

# Kerakli kutubxonalarni o'rnatish
RUN pip install --no-cache-dir -r requirements.txt

# Ilovangizning barcha fayllarini konteynerga nusxalash
COPY . .

# Konteynerda ishlatiladigan portni ochish
EXPOSE 8000

# Flask ilovasini Gunicorn yordamida ishga tushurish
CMD ["gunicorn", "-b", "0.0.0.0:8000", "app:app"]
