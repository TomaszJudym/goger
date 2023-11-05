from selenium import webdriver
from selenium.webdriver import Chrome
from selenium.webdriver.common.by import By
from typing import List
from pg import Repo
import concurrent.futures
import time
from threading import Lock

class FileWrapper:
    def __init__(self, file_path, mode="r"):
        self.mode = mode
        self.file = open(file_path, self.mode)
        self.lock = Lock()

    def __enter__(self):
        return self.file

    def write(self, s: List[str]):
        with self.lock:
            for line in s:
                self.file.write(line + '\n')

    def __exit__(self, exc_type, exc_value, traceback):
        if self.file is not None:
            self.file.close()

def retry(func, max_retries, base_retry_interval=2, max_retry_interval=60):
    """
    Retry the given function with exponential backoff.

    Args:
        func: The function to be retried.
        max_retries: The maximum number of retries.
        base_retry_interval: The initial retry interval (in seconds).
        max_retry_interval: The maximum retry interval (in seconds).

    Returns:
        The return value of the function on success, or None if all retries fail.
    """
    retry_count = 0
    retry_interval = base_retry_interval

    while retry_count < max_retries:
        try:
            result = func()  # Call the provided function
            return result  # Return the result on success
        except Exception as e:
            retry_count += 1
            if retry_count < max_retries:
                print(f"Retrying ({retry_count}/{max_retries}) in {retry_interval} seconds after: {e}")
                time.sleep(retry_interval)
                # Exponential backoff: double the retry_interval, but capped at max_retry_interval
                retry_interval = min(2 * retry_interval, max_retry_interval)

    print(f"Function failed after {max_retries} retries.")
    raise Exception(f"Retry failed after {max_retries} times")


def get_games_links(driver: Chrome, page: int=0) -> [str]:
    # URL of the web page to scrap
    url = 'https://www.gog.com/en/games'
    if page != 0:
        page = url + f"?page={page}"

    # Load the web page
    driver.get(url)

    links = []
    elems = driver.find_elements(By.CSS_SELECTOR, 'a[href^="https://www.gog.com/en/game"]')
    for elem in elems:
        link = elem.get_attribute('href')
        if link:
            links.append(link)
        else:
            print(f"href not found in element with text: {elem.text}")

    return links


def get_last_games_page_index(driver: Chrome) -> int:
    elements = driver.find_elements(By.CSS_SELECTOR, 'button.pagination__item.ng-star-inserted')
    val_str = elements[-1].text
    try:
        val_int = int(val_str)
    except ValueError:
        print(f"Wanted int index got: {val_str} of html element: {elements[-1]}")
        return -1
    return val_int

def new_chrome_driver() -> Chrome:
    # Configure ChromeOptions for incognito and headless mode
    options = webdriver.ChromeOptions()
    options.add_argument('--no-sandbox')
    options.add_argument("--incognito")
    options.add_argument("--headless")
#    options.add_argument('--disable-dev-shm-usage')
    options.add_argument('--disable-gpu')
    options.add_argument("--disable-setuid-sandbox")
    options.add_experimental_option("prefs", {"profile.managed_default_content_settings.images": 2})
    driver = webdriver.Chrome(options=options) 
    driver.implicitly_wait(60)
    return driver


def scrap_game_links(driver: Chrome, page_from: int, page_to: int,
     to: FileWrapper, repo: Repo) -> int:
    fetched = 0
    total = page_to - page_from + 1
    i = 0
    while page_from <= page_to:
        # Get links from first page and check how many pages there's in total.
        game_hrefs = retry(lambda: get_games_links(driver, page_from), 10)
        if len(game_hrefs) == 0:
            print(f"No game links found on page {i} - exiting")
            break 

        got = len(game_hrefs)
        fetched += got
        to.write(game_hrefs)
        for href in game_hrefs:
            repo.insert_game_link(href.rsplit('/', 1)[-1], href)
        page_from += 1
        i += 1
        print(f"Fetched {got} new links, going to page {page_from} {i}/{total}")

    driver.quit()
    return fetched


def generate_ranges(start, end, num_ranges):
    range_size = (end - start) // num_ranges
    ranges = []
    remainder = (end - start) % num_ranges
    next_start = start

    for i in range(num_ranges):
        range_start = next_start
        if remainder > 0:
            next_start += range_size + 1
            remainder -= 1
        else:
            next_start += range_size
        range_end = min(next_start - 1, end)
        ranges.append([range_start, range_end])

    return ranges


def main():
    repo = Repo()
    repo.ping()
    print("Pinged repo successfully")
    repo.clear("game_links")
    repo.clear("reviews")

    driver = new_chrome_driver()
    links_file = "gog_games_hrefs.txt"
    total = 0

    game_hrefs = retry(lambda: get_games_links(driver, 0), 10)
    if len(game_hrefs) == 0:
        print("No game links found on first page - exiting")
        return 
    total += len(game_hrefs)
    # Save found files
    f = FileWrapper(links_file, mode="w")
    f.write(game_hrefs)

    # First page is fetched, new workers start from 2nd
    start_index = 2
    # Driver is on first page, check how many pages there're in total.
    print("Fetching first page")
    start = time.time()
    last_index = get_last_games_page_index(driver)
    if last_index == -1:
        print("Failed to find last games list button index, exiting")
        return
    # Split into 4 ranges of pages and run in parallel
    workers_count = 4
    # List of ranges for each worker.
    ranges = generate_ranges(start_index, last_index, workers_count)
    pages_per_worker = ranges[0][1] - ranges[0][0]
    print(f"Running {workers_count} workers each fetching up to: {pages_per_worker} pages")

    with concurrent.futures.ThreadPoolExecutor() as executor:
        futures = []
        # Got 1 driver from fetching first page. Run it as first worker and then
        # spawn remaining workers with new drivers.
        print(f"Running worker 0 with range {ranges[0][0]} - {ranges[0][1]}")
        futures.append(executor.submit(scrap_game_links,
                        new_chrome_driver(), ranges[0][0], ranges[0][1], f, repo))

        for i, r in enumerate(ranges[1:]):
            print(f"Running worker {i} with range {r[0]} - {r[1]}")
            futures.append(executor.submit(scrap_game_links,
                            new_chrome_driver(), r[0], r[1], f, repo))

    # Collect the results when all tasks are completed
    total = []
    for f in futures:
        total.append(f.result())
        print(f"Results fetched by worker: {f.result()}")
    
    print(f"Fetched in total {sum(total)} game links in {time.time() - start: .2f}s")

# Check if this script is the main program (not imported as a module)
if __name__ == "__main__":
    main()
