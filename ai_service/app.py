from flask import Flask, request, jsonify
import os
import uuid
from model_ai import load_model, preprocess_png, predict_and_save_masks
import torch


app = Flask(__name__, static_folder='uploads', static_url_path='/files')     
UPLOAD_FOLDER = '/app/uploads'
PREDICTION_FOLDER = '/app/predictions'
MODEL_PATH = 'laser_model_mk2.pth'

os.makedirs(UPLOAD_FOLDER, exist_ok=True)
os.makedirs(PREDICTION_FOLDER, exist_ok=True)

device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
model = load_model(MODEL_PATH)
model.to(device)

@app.route('/predict', methods=['POST'])
def predict():
    if 'file' not in request.files:
        return jsonify({"error": "No file provided"}), 400

    file = request.files['file']
    if not file.filename.endswith('.png'):
        return jsonify({"error": "Only .png files are supported"}), 400

    filename = f"{uuid.uuid4().hex}.png"
    file_path = os.path.join(UPLOAD_FOLDER, filename)
    file.save(file_path)

    image_tensor = preprocess_png(file_path).to(device)
    prediction = predict_and_save_masks(model, image_tensor, UPLOAD_FOLDER, filename[:-4])

    x_mm, y_mm, angle_deg = prediction
    return jsonify({
        "x_mm": round(float(x_mm), 2),
        "y_mm": round(float(y_mm), 2),
        "angle_deg": round(float(angle_deg), 2),
        "segmentation_masks": [
            f"/files/{filename[:-4]}_class{i}.png" for i in range(3)
        ]
    })

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=5000, debug=True)