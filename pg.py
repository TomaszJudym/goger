import psycopg2
import os

class Repo:
    def __init__(self):
        db_params = {
            "dbname": os.environ.get("POSTGRES_DB"),
            "user": os.environ.get("POSTGRES_USER"),
            "password": os.environ.get("POSTGRES_PASSWORD"),
            "host": os.environ.get("POSTGRES_HOST"),
            "port": os.environ.get("POSTGRES_PORT")
        }
        self.conn = psycopg2.connect(**db_params)
        self.cur = self.conn.cursor()
        self.create_tables()

    def create_tables(self):
        create_game_links_table = """
            CREATE TABLE IF NOT EXISTS game_links (
                ID serial PRIMARY KEY,
                name TEXT,
                link TEXT
            );
        """
        create_reviews_table = """
            CREATE TABLE IF NOT EXISTS reviews (
                ID serial PRIMARY KEY,
                game_id serial REFERENCES game_links(ID),
                text TEXT
            );
        """
        self.cur.execute(create_game_links_table)
        self.cur.execute(create_reviews_table)
        self.conn.commit()

    def ping(self):
        try:
            self.cur.execute("SELECT 1")
            self.conn.commit()
        except Exception as e:
            print(f"Error pinging the database: {str(e)}")

    def clear(self, table):
        try:
            self.cur.execute(f"TRUNCATE {table} CASCADE")
            self.conn.commit()
        except Exception as e:
            print(f"Error truncating table {table}: {str(e)}")

    def insert_game_link(self, name, link):
        try:
            self.cur.execute("INSERT INTO game_links (name, link) VALUES (%s, %s)", (name, link))
            self.conn.commit()
        except Exception as e:
            print(f"Error inserting game link: {str(e)}")

    def insert_review(self, game_id, text):
        try:
            self.cur.execute("INSERT INTO reviews (game_id, text) VALUES (%s, %s)", (game_id, text))
            self.conn.commit()
        except Exception as e:
            print(f"Error inserting review: {str(e)}")

    def __del__(self):
        self.cur.close()
        self.conn.close()
