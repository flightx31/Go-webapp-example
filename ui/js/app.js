const app = function(){
    const appElement = document.getElementById("app");
    const child = document.createElement("div")
    child.textContent = "This is a child element added to the app div using javascript";
    appElement.appendChild(child);
};

window.addEventListener('DOMContentLoaded', (event) => {
    app();
});